package qa

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"venturo-skeleton-go/internal/config"
)

func TestServerRealRoutes(t *testing.T) {
	if os.Getenv("AI_CREDENTIAL_ENCRYPTION_KEY") == "" {
		t.Skip("QA database/JWT environment not loaded")
	}
	cfg := config.Load()
	pool, err := pgxpool.New(context.Background(), cfg.Database.GetDSN())
	require.NoError(t, err)
	defer pool.Close()
	require.NoError(t, pool.Ping(context.Background()))
	srv, err := NewServer(pool, filepath.Join("..", "service", "testdata", "blueprint_http", "queue.json"))
	require.NoError(t, err)
	defer func() { require.NoError(t, srv.Close()) }()
	call := func(method, path string, body []byte) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+srv.token)
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)
		return w
	}
	session := call("GET", "/qa/session", nil)
	require.Equal(t, 200, session.Code)
	require.NotContains(t, session.Body.String(), "AI_CREDENTIAL")
	require.Equal(t, 200, call("GET", "/quotation/v1/quotations", nil).Code)
	require.Equal(t, 200, call("GET", "/quotation/v1/quotations/"+srv.QuotationID.String()+"/revisions", nil).Code)
	require.Equal(t, 200, call("GET", "/quotation/v1/blueprints", nil).Code)
	input := map[string]any{"idempotency_key": uuid.NewString(), "quotation_id": srv.QuotationID, "source_quotation_revision_id": srv.RevisionID, "title": "Sistem Antrian Pelayanan Terpadu", "business_goals": []string{"Reduce wait time"}, "actors": []string{"Applicant"}, "channels": []string{"Kiosk"}, "business_phases": []string{"Registration"}}
	body, err := json.Marshal(input)
	require.NoError(t, err)
	generated := call("POST", "/quotation/v1/blueprints/generate", body)
	require.Equal(t, 201, generated.Code, generated.Body.String())
	var count int
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT count(*) FROM blueprints WHERE company_id=$1`, srv.CompanyID).Scan(&count))
	require.Equal(t, 1, count)
	var blueprintID uuid.UUID
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT id FROM blueprints WHERE company_id=$1`, srv.CompanyID).Scan(&blueprintID))
	canary := call("GET", "/qa/canary/"+blueprintID.String(), nil)
	require.Equal(t, 200, canary.Code)
	require.JSONEq(t, `{"source_seeded":true,"skill_seeded":true,"source_snapshotted":true,"skill_snapshotted":true}`, canary.Body.String())
	for _, marker := range []string{commercialCanary, legalCanary, credentialCanary, promptCanary} {
		require.NotContains(t, generated.Body.String(), marker)
		require.NotContains(t, canary.Body.String(), marker)
	}
}
