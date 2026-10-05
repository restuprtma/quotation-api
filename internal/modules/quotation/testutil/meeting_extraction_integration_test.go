//go:build integration

package testutil

import "testing"

// This is intentionally a guard test rather than a migration test. A target
// without quotation_extraction_runs or with an old quotation ledger must fail
// in Pool with the actionable fatal message; a missing explicit DSN must skip.
func TestMeetingExtractionIntegrationDatabaseGuard(t *testing.T) {
	pool := Pool(t)
	if pool == nil {
		t.Fatal("meeting extraction integration guard returned a nil pool")
	}
}
