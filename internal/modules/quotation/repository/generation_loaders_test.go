//go:build integration

package repository_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"venturo-skeleton-go/internal/modules/quotation/repository"
)

const loaderLegalCode = "quotation_standard"

type loaderFixture struct {
	companyID          uuid.UUID
	otherCompanyID     uuid.UUID
	skillID            uuid.UUID
	inactiveSkillID    uuid.UUID
	rateCardID         uuid.UUID
	otherCompanyCardID uuid.UUID
}

func seedLoaderFixture(t *testing.T, pool *pgxpool.Pool) loaderFixture {
	t.Helper()
	fx := loaderFixture{
		companyID:          uuid.New(),
		otherCompanyID:     uuid.New(),
		skillID:            uuid.New(),
		inactiveSkillID:    uuid.New(),
		rateCardID:         uuid.New(),
		otherCompanyCardID: uuid.New(),
	}
	ctx := context.Background()

	_, err := pool.Exec(ctx, `
		INSERT INTO rate_cards (id, company_id, code, name, market, currency, effective_from, is_active)
		VALUES ($1, $2, 'RC-S4', 'Session4 Card', 'national', 'IDR', NOW(), true),
		       ($3, $4, 'RC-S4-OTHER', 'Other Company Card', 'national', 'IDR', NOW(), true)
	`, fx.rateCardID, fx.companyID, fx.otherCompanyCardID, fx.otherCompanyID)
	require.NoError(t, err)

	for _, item := range []struct {
		role, seniority, term string
		amount                int64
	}{
		{"programmer", "senior", "long_term", 12_000_000},
		{"business_analyst", "intermediate", "long_term", 9_000_000},
	} {
		_, err = pool.Exec(ctx, `
			INSERT INTO rate_card_items (company_id, rate_card_id, role, seniority, term_type, amount_per_month)
			VALUES ($1, $2, $3, $4, $5, $6)
		`, fx.companyID, fx.rateCardID, item.role, item.seniority, item.term, item.amount)
		require.NoError(t, err)
	}

	_, err = pool.Exec(ctx, `
		INSERT INTO legal_templates (id, company_id, code, version, content_html, is_active)
		VALUES ($1, $2, $3, 1, '<p>Syarat versi satu.</p>', true),
		       ($4, $2, $3, 2, '<p>Syarat versi dua.</p>', true)
	`, uuid.New(), fx.companyID, loaderLegalCode, uuid.New())
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `
		INSERT INTO ai_skills (id, company_id, name, command, content_instruction, is_active)
		VALUES ($1, $2, 'Quotation S4', 'quotation_s4', 'Susun alokasi dedicated squad.', true),
		       ($3, $2, 'Quotation S4 Off', 'quotation_s4_off', 'Nonaktif.', false)
	`, fx.skillID, fx.companyID, fx.inactiveSkillID)
	require.NoError(t, err)

	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*1e9)
		defer cancel()
		for _, companyID := range []uuid.UUID{fx.companyID, fx.otherCompanyID} {
			pool.Exec(cleanupCtx, `UPDATE quotations SET active_published_revision_id = NULL WHERE company_id = $1`, companyID)
			pool.Exec(cleanupCtx, `DELETE FROM quotation_revisions WHERE company_id = $1`, companyID)
			pool.Exec(cleanupCtx, `DELETE FROM quotations WHERE company_id = $1`, companyID)
			pool.Exec(cleanupCtx, `DELETE FROM rate_card_items WHERE company_id = $1`, companyID)
			pool.Exec(cleanupCtx, `DELETE FROM rate_cards WHERE company_id = $1`, companyID)
			pool.Exec(cleanupCtx, `DELETE FROM legal_templates WHERE company_id = $1`, companyID)
			pool.Exec(cleanupCtx, `DELETE FROM ai_skills WHERE company_id = $1`, companyID)
		}
	})

	return fx
}

func TestGenerationLoaders(t *testing.T) {
	pool := getTestPool(t)
	fx := seedLoaderFixture(t, pool)
	repo := repository.NewGenerationRepository(pool)
	ctx := context.Background()

	t.Run("GetRateCardWithItems_ReturnsHeaderAndItems", func(t *testing.T) {
		card, items, err := repo.GetRateCardWithItems(ctx, fx.companyID, fx.rateCardID)
		require.NoError(t, err)
		assert.Equal(t, "RC-S4", card.Code)
		assert.Equal(t, "IDR", card.Currency)
		assert.True(t, card.IsActive)
		require.Len(t, items, 2)
		roles := map[string]int64{}
		for _, item := range items {
			roles[item.Role] = item.AmountPerMonth
		}
		assert.Equal(t, int64(12_000_000), roles["programmer"])
		assert.Equal(t, int64(9_000_000), roles["business_analyst"])
	})

	t.Run("GetRateCardWithItems_TenantIsolated", func(t *testing.T) {
		_, _, err := repo.GetRateCardWithItems(ctx, fx.otherCompanyID, fx.rateCardID)
		assert.ErrorIs(t, err, repository.ErrRateCardNotFound)
	})

	t.Run("GetRateCardWithItems_MissingID", func(t *testing.T) {
		_, _, err := repo.GetRateCardWithItems(ctx, fx.companyID, uuid.New())
		assert.ErrorIs(t, err, repository.ErrRateCardNotFound)
	})

	t.Run("GetSkillForGeneration_ReturnsActiveSkill", func(t *testing.T) {
		skill, err := repo.GetSkillForGeneration(ctx, fx.companyID, fx.skillID)
		require.NoError(t, err)
		assert.Equal(t, "quotation_s4", skill.Command)
		assert.Equal(t, "Susun alokasi dedicated squad.", skill.ContentInstruction)
	})

	t.Run("GetSkillForGeneration_InactiveRejected", func(t *testing.T) {
		_, err := repo.GetSkillForGeneration(ctx, fx.companyID, fx.inactiveSkillID)
		assert.ErrorIs(t, err, repository.ErrSkillNotFound)
	})

	t.Run("GetSkillForGeneration_TenantIsolated", func(t *testing.T) {
		_, err := repo.GetSkillForGeneration(ctx, fx.otherCompanyID, fx.skillID)
		assert.ErrorIs(t, err, repository.ErrSkillNotFound)
	})

	t.Run("GetActiveLegalTemplate_ReturnsHighestVersionActive", func(t *testing.T) {
		legal, err := repo.GetActiveLegalTemplateForCompany(ctx, fx.companyID, loaderLegalCode)
		require.NoError(t, err)
		assert.Equal(t, 2, legal.Version)
		assert.Contains(t, legal.ContentHTML, "versi dua")
	})

	t.Run("GetActiveLegalTemplate_UnknownCode", func(t *testing.T) {
		_, err := repo.GetActiveLegalTemplateForCompany(ctx, fx.companyID, "legal-tidak-ada")
		assert.ErrorIs(t, err, repository.ErrLegalTemplateNotFound)
	})

	t.Run("GetClientName_MissReturnsEmptyWithoutError", func(t *testing.T) {
		name, err := repo.GetClientName(ctx, uuid.New())
		require.NoError(t, err)
		assert.Empty(t, name)
	})

	t.Run("NextQuotationNumber_IncrementsPerCompany", func(t *testing.T) {
		first, err := repo.NextQuotationNumber(ctx, fx.companyID)
		require.NoError(t, err)
		assert.Regexp(t, `^QUO-\d{4}-\d{4}$`, first)

		_, err = pool.Exec(ctx, `
			INSERT INTO quotations (id, company_id, client_id, quotation_number, solution_type, commercial_scheme, status, current_revision)
			VALUES ($1, $2, $3, $4, 'custom_software', 'dedicated_squad', 'queued', 1)
		`, uuid.New(), fx.companyID, uuid.New(), first)
		require.NoError(t, err)

		second, err := repo.NextQuotationNumber(ctx, fx.companyID)
		require.NoError(t, err)
		assert.NotEqual(t, first, second)
	})

	t.Run("Pool_ReturnsUnderlyingPool", func(t *testing.T) {
		assert.NotNil(t, repo.Pool())
	})
}
