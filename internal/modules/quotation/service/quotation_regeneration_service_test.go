//go:build integration

package service_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"venturo-skeleton-go/internal/modules/quotation/repository"
	"venturo-skeleton-go/internal/modules/quotation/service"
)

// revisionSnapshot menangkap state persist yang relevan untuk verifikasi
// atomicity regeneration.
type revisionSnapshot struct {
	status    string
	html      string
	version   int
	regenCnt  int
	number    string
	revisions int
}

func captureSnapshot(t *testing.T, pool *pgxpool.Pool, quotationID uuid.UUID) revisionSnapshot {
	t.Helper()
	var snap revisionSnapshot
	require.NoError(t, pool.QueryRow(context.Background(), `
		SELECT r.generation_status,
		       COALESCE(r.content_html, ''),
		       r.content_version,
		       r.regenerate_count,
		       q.quotation_number,
		       (SELECT COUNT(*) FROM quotation_revisions WHERE quotation_id = q.id)
		FROM quotation_revisions r
		JOIN quotations q ON q.id = r.quotation_id
		WHERE r.quotation_id = $1
	`, quotationID).Scan(&snap.status, &snap.html, &snap.version, &snap.regenCnt, &snap.number, &snap.revisions))
	return snap
}

func generateOnce(t *testing.T, pool *pgxpool.Pool) (uuid.UUID, *scriptedAIProvider, pipelineFixture) {
	t.Helper()
	fx := seedPipelineFixtures(t, pool)
	mock := &scriptedAIProvider{steps: []scriptedStep{
		{text: allocationJSON()},
		{text: narrativeHTML()},
	}}
	svc, _ := newSvc(pool, mock)
	qid, err := svc.Generate(context.Background(), fx.companyID, nil, fx.request())
	require.NoError(t, err)
	return qid, mock, fx
}

// companyOf membaca pemilik quotation dari DB untuk skenario yang hanya
// membawa quotation ID (meniru perilaku handler yang mengambil company dari
// JWT context).
func companyOf(t *testing.T, pool *pgxpool.Pool, quotationID uuid.UUID) uuid.UUID {
	t.Helper()
	var companyID uuid.UUID
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT company_id FROM quotations WHERE id = $1`, quotationID).Scan(&companyID))
	return companyID
}

func TestRegenerate_HappyPath_ReplacesHTMLIncrementsVersionSingleRevision(t *testing.T) {
	pool := getTestPool(t)
	qid, _, _ := generateOnce(t, pool)
	before := captureSnapshot(t, pool, qid)

	// Mock terpisah untuk fase regenerate: alokasi sama, naratif berbeda.
	regenMock := &scriptedAIProvider{steps: []scriptedStep{
		{text: allocationJSON()},
		{text: `<section data-block-type="opening"><p>Narasi hasil regenerasi</p><ul><li>Scope diperbarui</li></ul></section>`},
	}}
	svc, _ := newSvc(pool, regenMock)

	require.NoError(t, svc.Regenerate(context.Background(), companyOf(t, pool, qid), qid))

	after := captureSnapshot(t, pool, qid)
	assert.Equal(t, "completed", after.status)
	assert.Contains(t, after.html, "Narasi hasil regenerasi")
	assert.NotContains(t, after.html, "Kami menyampaikan penawaran",
		"narrative lama harus tergantikan penuh")
	assert.Equal(t, before.version+1, after.version, "regenerate sukses wajib increment content_version")
	assert.Equal(t, 1, after.revisions, "regenerate TIDAK boleh membuat revision baru")
	assert.Equal(t, before.number, after.number, "quotation_number wajib stabil")
	assert.Equal(t, 1, after.regenCnt, "satu regenerate sukses = counter naik ke 1")
}

func TestRegenerate_NarrativeTimeout_OldContentIntactVersionUnchanged(t *testing.T) {
	pool := getTestPool(t)
	qid, _, _ := generateOnce(t, pool)
	before := captureSnapshot(t, pool, qid)

	// Allocation cepat, narrative melewati deadline attempt => fatal timeout.
	regenMock := &scriptedAIProvider{steps: []scriptedStep{
		{text: allocationJSON()},
		{text: `<p>terlambat</p>`, delay: 500 * time.Millisecond},
	}}
	svc, _ := newSvc(pool, regenMock, service.WithAttemptTimeout(80*time.Millisecond))

	err := svc.Regenerate(context.Background(), companyOf(t, pool, qid), qid)
	require.Error(t, err)
	var ge *service.GenerationError
	require.ErrorAs(t, err, &ge)
	assert.Equal(t, "provider_timeout", ge.Code)

	after := captureSnapshot(t, pool, qid)
	assert.Equal(t, before.html, after.html, "content_html lama WAJIB utuh byte-per-byte")
	assert.Equal(t, before.version, after.version, "gagal regenerate TIDAK boleh increment content_version")
	assert.Equal(t, 1, after.revisions)
	assert.Equal(t, 1, after.regenCnt, "quota tetap terpakai meski AI gagal (guard membatasi spend)")
}

func TestRegenerate_UsesSavedGenerationInput_StableQuotationNumber(t *testing.T) {
	pool := getTestPool(t)
	qid, _, fx := generateOnce(t, pool)
	before := captureSnapshot(t, pool, qid)

	regenMock := &scriptedAIProvider{steps: []scriptedStep{
		{text: allocationJSON()},
		{text: `<section data-block-type="opening"><p>Narasi regenerasi input tersimpan</p></section>`},
	}}
	svc, _ := newSvc(pool, regenMock)

	require.NoError(t, svc.Regenerate(context.Background(), fx.companyID, qid))

	require.GreaterOrEqual(t, len(regenMock.prompts), 2)
	// Prompt alokasi membawa generation_input TERSIMPAN.
	assert.Contains(t, regenMock.prompts[0], "Pengembangan Customer Portal")
	assert.Contains(t, regenMock.prompts[0], "Customer portal")
	// Nomor quotation dipakai ulang dari master, bukan sequence baru.
	assert.Contains(t, regenMock.prompts[1], before.number)

	after := captureSnapshot(t, pool, qid)
	assert.Contains(t, after.html, before.number,
		"metadata block harus tetap menampilkan nomor quotation asli")
	assert.Equal(t, before.number, after.number)
}

func TestRegenerate_NotDraft_RevisionConflict(t *testing.T) {
	pool := getTestPool(t)
	qid, _, fx := generateOnce(t, pool)

	_, err := pool.Exec(context.Background(),
		`UPDATE quotation_revisions SET publication_status = 'published' WHERE quotation_id = $1`, qid)
	require.NoError(t, err)

	svc, _ := newSvc(pool, &scriptedAIProvider{})
	err = svc.Regenerate(context.Background(), fx.companyID, qid)
	require.Error(t, err)
	var ge *service.GenerationError
	require.ErrorAs(t, err, &ge)
	assert.Equal(t, "revision_conflict", ge.Code)
}

func TestRegenerate_ForeignOrMissingQuotation_NotFoundPassthrough(t *testing.T) {
	pool := getTestPool(t)
	qid, _, _ := generateOnce(t, pool)

	svc, _ := newSvc(pool, &scriptedAIProvider{})

	err := svc.Regenerate(context.Background(), uuid.New(), qid)
	require.Error(t, err)
	assert.True(t, errors.Is(err, repository.ErrRevisionNotFound),
		"company lain harus menerima not-found (404-equivalent)")

	err = svc.Regenerate(context.Background(), uuid.New(), uuid.New())
	assert.True(t, errors.Is(err, repository.ErrRevisionNotFound))
}

func TestRegenerate_QuotaExhausted_RateLimited_NoProviderCall(t *testing.T) {
	pool := getTestPool(t)
	ctx := context.Background()
	qid, _, fx := generateOnce(t, pool)
	before := captureSnapshot(t, pool, qid)

	_, err := pool.Exec(ctx, `
		UPDATE quotation_revisions
		SET regenerate_count = $2, last_regenerate_at = NOW() - INTERVAL '10 minutes'
		WHERE quotation_id = $1
	`, qid, repository.MaxRegeneratesPerHour)
	require.NoError(t, err)

	regenMock := &scriptedAIProvider{}
	svc, _ := newSvc(pool, regenMock)

	err = svc.Regenerate(ctx, fx.companyID, qid)
	assertGenerationCode(t, err, "regenerate_rate_limited")
	assert.Zero(t, regenMock.callCount(), "limit habis tidak boleh menyentuh provider")

	after := captureSnapshot(t, pool, qid)
	assert.Equal(t, before.html, after.html)
	assert.Equal(t, before.version, after.version)
	assert.Equal(t, repository.MaxRegeneratesPerHour, after.regenCnt,
		"penolakan tidak boleh menaikkan counter")
}

func TestRegenerate_MissingSkill_SettingsIncomplete_OldContentIntact(t *testing.T) {
	pool := getTestPool(t)
	ctx := context.Background()
	qid, _, fx := generateOnce(t, pool)
	before := captureSnapshot(t, pool, qid)

	_, err := pool.Exec(ctx, `DELETE FROM ai_skills WHERE id = $1`, fx.skillID)
	require.NoError(t, err)

	regenMock := &scriptedAIProvider{}
	svc, _ := newSvc(pool, regenMock)

	err = svc.Regenerate(ctx, fx.companyID, qid)
	assertGenerationCode(t, err, "settings_incomplete")

	after := captureSnapshot(t, pool, qid)
	assert.Equal(t, before.html, after.html, "kegagalan konfigurasi tidak boleh merusak konten")
	assert.Equal(t, before.version, after.version)
}

func TestRegenerate_UnsafeNarrative_OldContentIntactVersionUnchanged(t *testing.T) {
	pool := getTestPool(t)
	qid, _, fx := generateOnce(t, pool)
	before := captureSnapshot(t, pool, qid)

	// Narrative membocorkan budget maximum => DOM validator menolak di tahap akhir.
	regenMock := &scriptedAIProvider{steps: []scriptedStep{
		{text: allocationJSON()},
		{text: `<section data-block-type="opening"><p>Total penawaran kami hanya Rp 150.000.000 semua termasuk.</p></section>`},
	}}
	svc, _ := newSvc(pool, regenMock)

	err := svc.Regenerate(context.Background(), fx.companyID, qid)
	assertGenerationCode(t, err, "unsafe_html")

	after := captureSnapshot(t, pool, qid)
	assert.True(t, strings.Contains(after.html, "Rp 63.000.000"), "konten lama masih tampil")
	assert.Equal(t, before.version, after.version)
}

func TestRegenerate_StandaloneMaintenance_HappyPath_ReplacesHTMLIncrementsVersionSingleRevision(t *testing.T) {
	pool := getTestPool(t)
	fx := seedPipelineFixtures(t, pool)

	mock := &panicAIProvider{}
	svc, source := newSvc(pool, mock)

	input := nexRetainerInputForCalculator()
	req := standaloneMaintenanceRequest(fx, input)

	qid, err := svc.Generate(context.Background(), fx.companyID, nil, req)
	require.NoError(t, err)

	before := captureSnapshot(t, pool, qid)
	assert.Equal(t, "completed", before.status)
	assert.Equal(t, 1, before.version)

	err = svc.Regenerate(context.Background(), fx.companyID, qid)
	require.NoError(t, err)

	assert.Empty(t, source.requests(), "AI provider must not be called during standalone maintenance regenerate")

	after := captureSnapshot(t, pool, qid)
	assert.Equal(t, "completed", after.status)
	assert.Equal(t, before.version+1, after.version, "regenerate must increment content_version")
	assert.Equal(t, 1, after.revisions, "regenerate must not create a new revision row")
	assert.Equal(t, before.number, after.number, "quotation_number must be preserved")
}

func TestRegenerateKioskQueueUsesCommittedSnapshots(t *testing.T) {
	pool := getTestPool(t)
	qid, companyID, _ := generateKioskQueueDraft(t, pool)

	before := captureSnapshot(t, pool, qid)
	assert.Equal(t, "completed", before.status)
	assert.Equal(t, 1, before.version)

	svc, _ := newSvc(pool, &panicAIProvider{})
	err := svc.Regenerate(context.Background(), companyID, qid)
	require.NoError(t, err)

	after := captureSnapshot(t, pool, qid)
	assert.Equal(t, "completed", after.status)
	assert.Equal(t, before.version+1, after.version, "regenerate must increment content_version")
	assert.Equal(t, 1, after.revisions, "regenerate must not create a new revision row")
	assert.Equal(t, before.number, after.number, "quotation_number must be preserved")
	assert.Equal(t, 1, after.regenCnt, "regenerate count must be 1")
}

func TestRegenerateKioskQueuePreservesDocumentNumber(t *testing.T) {
	pool := getTestPool(t)
	qid, companyID, _ := generateKioskQueueDraft(t, pool)

	before := captureSnapshot(t, pool, qid)

	svc, _ := newSvc(pool, &panicAIProvider{})
	err := svc.Regenerate(context.Background(), companyID, qid)
	require.NoError(t, err)

	after := captureSnapshot(t, pool, qid)
	assert.Equal(t, before.number, after.number)
}

func TestRegenerateKioskQueueDoesNotCallProvider(t *testing.T) {
	pool := getTestPool(t)
	qid, companyID, _ := generateKioskQueueDraft(t, pool)

	mock := &panicAIProvider{}
	svc, source := newSvc(pool, mock)

	err := svc.Regenerate(context.Background(), companyID, qid)
	require.NoError(t, err)

	assert.Empty(t, source.requests(), "AI provider must not be called during kiosk queue regenerate")
}

func TestRegenerateKioskQueueRejectsCorruptCalculationSnapshot(t *testing.T) {
	pool := getTestPool(t)
	ctx := context.Background()
	qid, companyID, _ := generateKioskQueueDraft(t, pool)
	before := captureSnapshot(t, pool, qid)

	_, err := pool.Exec(ctx, `UPDATE quotation_revisions SET calculation_snapshot = '{"total_investment":-1}' WHERE quotation_id = $1`, qid)
	require.NoError(t, err)

	svc, _ := newSvc(pool, &panicAIProvider{})
	err = svc.Regenerate(ctx, companyID, qid)
	require.Error(t, err)
	assertGenerationCode(t, err, "generation_failed")

	after := captureSnapshot(t, pool, qid)
	assert.Equal(t, before.html, after.html)
	assert.Equal(t, before.version, after.version)
}

func TestRegenerateKioskQueueFailurePreservesPreviousContentAndVersion(t *testing.T) {
	pool := getTestPool(t)
	qid, companyID, _ := generateKioskQueueDraft(t, pool)
	before := captureSnapshot(t, pool, qid)

	svc, _ := newSvc(pool, &panicAIProvider{}, service.WithKioskQueueRenderer(&failingKioskQueueRenderer{}))
	err := svc.Regenerate(context.Background(), companyID, qid)
	require.Error(t, err)
	assertGenerationCode(t, err, "unsafe_html")

	after := captureSnapshot(t, pool, qid)
	assert.Equal(t, before.html, after.html)
	assert.Equal(t, before.version, after.version)
}
