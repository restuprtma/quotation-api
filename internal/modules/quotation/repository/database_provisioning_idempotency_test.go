package repository_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDatabaseProvisioning_MigrationsAndSeedersIntegrity(t *testing.T) {
	root := findProjectRoot(t)

	// 1. Verify migrations directories exist
	modules := []string{"core", "quotation", "contract"}
	for _, mod := range modules {
		dir := filepath.Join(root, "internal", "database", "migrations", mod)
		info, err := os.Stat(dir)
		require.NoError(t, err, "migration directory for %s must exist", mod)
		assert.True(t, info.IsDir(), "%s must be a directory", dir)

		// Check up/down pairs
		entries, err := os.ReadDir(dir)
		require.NoError(t, err)

		files := make(map[string]bool)
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
				files[e.Name()] = true
			}
		}

		for f := range files {
			if strings.HasSuffix(f, ".up.sql") {
				downFile := strings.TrimSuffix(f, ".up.sql") + ".down.sql"
				assert.True(t, files[downFile], "missing down migration for %s in module %s", f, mod)
			}
		}
	}

	// 2. Verify seeders integrity
	seederQuotation := filepath.Join(root, "internal", "database", "seeders", "quotation", "001_venturo_master_rate_cards.sql")
	content, err := os.ReadFile(seederQuotation)
	require.NoError(t, err, "master rate cards seeder must exist")
	sqlStr := string(content)

	assert.Contains(t, sqlStr, "rate_cards", "seeder must populate rate_cards")
	assert.Contains(t, sqlStr, "rate_card_items", "seeder must populate rate_card_items")
	assert.Contains(t, sqlStr, "DEFAULT", "seeder must include DEFAULT card")
	assert.Contains(t, sqlStr, "VENTURO-STD-2026", "seeder must include VENTURO-STD-2026 card")
	assert.Contains(t, sqlStr, "VENTURO-INTL-2026", "seeder must include VENTURO-INTL-2026 card")
}
