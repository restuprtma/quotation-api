package repository_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"venturo-skeleton-go/internal/modules/quotation/domain"
)

func TestMigrationFiles_BlueprintShareLinks(t *testing.T) {
	root := findProjectRoot(t)

	upFile := filepath.Join(root, "internal", "database", "migrations", "quotation", "20261006120000_blueprint_share_links.up.sql")
	dnFile := filepath.Join(root, "internal", "database", "migrations", "quotation", "20261006120000_blueprint_share_links.down.sql")

	t.Run("up_migration_file", func(t *testing.T) {
		content, err := os.ReadFile(upFile)
		require.NoError(t, err, "up migration file must exist")
		sql := string(content)

		assert.Contains(t, sql, "CREATE TABLE IF NOT EXISTS blueprint_share_links")
		assert.Contains(t, sql, "uq_blueprint_share_links_token_hash_active")
		assert.Contains(t, sql, "idx_blueprint_share_links_blueprint_company")
		assert.Contains(t, sql, "idx_blueprint_share_links_token_hash")
		assert.True(t, strings.Contains(sql, "REFERENCES blueprints(id)") || strings.Contains(sql, "REFERENCES blueprints (id)"))
		assert.True(t, strings.Contains(sql, "REFERENCES blueprint_revisions(id)") || strings.Contains(sql, "REFERENCES blueprint_revisions (id)"))
	})

	t.Run("down_migration_file", func(t *testing.T) {
		content, err := os.ReadFile(dnFile)
		require.NoError(t, err, "down migration file must exist")
		sql := string(content)

		assert.Contains(t, sql, "DROP TABLE IF EXISTS blueprint_share_links")
		assert.Contains(t, sql, "DROP INDEX IF EXISTS uq_blueprint_share_links_token_hash_active")
	})

	t.Run("domain_share_link_methods", func(t *testing.T) {
		now := time.Now()
		activeLink := domain.ShareLink{
			ID:                  uuid.New(),
			CompanyID:           uuid.New(),
			BlueprintID:         uuid.New(),
			BlueprintRevisionID: uuid.New(),
			TokenHash:           "hash123",
			Label:               "Test Share Link",
			CreatedAt:           now,
			RevokedAt:           nil,
		}

		assert.True(t, activeLink.IsActive())
		assert.False(t, activeLink.IsRevoked())

		revokedLink := domain.ShareLink{
			ID:                  uuid.New(),
			CompanyID:           uuid.New(),
			BlueprintID:         uuid.New(),
			BlueprintRevisionID: uuid.New(),
			TokenHash:           "hash456",
			Label:               "Revoked Share Link",
			CreatedAt:           now,
			RevokedAt:           &now,
		}

		assert.False(t, revokedLink.IsActive())
		assert.True(t, revokedLink.IsRevoked())
	})
}
