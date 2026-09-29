package repository_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"venturo-skeleton-go/internal/modules/quotation/domain"
	"venturo-skeleton-go/internal/modules/quotation/repository"
)

type revisionPromptCaptureExecer struct {
	query string
	args  []any
}

func (e *revisionPromptCaptureExecer) QueryRow(_ context.Context, query string, args ...any) pgx.Row {
	e.query = query
	e.args = args
	return revisionPromptCaptureRow{}
}

func (e *revisionPromptCaptureExecer) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, errors.New("not used by this test")
}

func (e *revisionPromptCaptureExecer) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, errors.New("not used by this test")
}

type revisionPromptCaptureRow struct{}

func (revisionPromptCaptureRow) Scan(...any) error { return nil }

func TestCreateRevision_PersistsPromptStateColumnsInOrder(t *testing.T) {
	state := []byte(`{"schema_version":"prompt_quotation_v1"}`)
	hashes := []byte(`{"scope":"sha256"}`)
	html := "<section>draft</section>"
	version := "1"
	revision := &domain.QuotationRevision{
		ID:                    uuid.New(),
		CompanyID:             uuid.New(),
		QuotationID:           uuid.New(),
		RevisionNumber:        1,
		PublicationStatus:     "draft",
		GenerationStatus:      "pending",
		LegalTemplateSnapshot: []byte(`{"version":1}`),
		PromptState:           state,
		PromptStateVersion:    7,
		PromptSectionHashes:   hashes,
		ContentHTML:           &html,
		ContentVersion:        &version,
	}
	capture := &revisionPromptCaptureExecer{}

	err := new(repository.RevisionRepository).CreateRevision(context.Background(), capture, revision)

	require.NoError(t, err)
	require.Len(t, capture.args, 38)
	assert.Equal(t, revision.PromptState, capture.args[12])
	assert.Equal(t, revision.PromptStateVersion, capture.args[13])
	assert.Equal(t, revision.PromptSectionHashes, capture.args[14])
	assert.Equal(t, revision.ContentHTML, capture.args[15])
	assert.Contains(t, strings.ReplaceAll(capture.query, "\n", " "), "$38")
}
