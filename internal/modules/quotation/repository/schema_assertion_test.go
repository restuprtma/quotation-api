//go:build integration

package repository_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testDB(t *testing.T) *pgxpool.Pool {
	return getTestPool(t)
}

func getTestPool(t *testing.T) *pgxpool.Pool {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		host := os.Getenv("DB_HOST")
		if host == "" {
			host = "localhost"
		}
		port := os.Getenv("DB_PORT")
		if port == "" {
			port = "5432"
		}
		user := os.Getenv("DB_USER")
		if user == "" {
			user = "postgres"
		}
		pass := os.Getenv("DB_PASSWORD")
		if pass == "" {
			pass = "postgres"
		}
		name := os.Getenv("DB_NAME")
		if name == "" {
			name = "quotation_db"
		}
		ssl := os.Getenv("DB_SSLMODE")
		if ssl == "" {
			ssl = "disable"
		}
		dsn = fmt.Sprintf("postgresql://%s:%s@%s:%s/%s?sslmode=%s", user, pass, host, port, name, ssl)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("failed to connect to test database: %v", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Fatalf("failed to ping test database: %v", err)
	}

	t.Cleanup(func() {
		pool.Close()
	})

	return pool
}

func tableExists(ctx context.Context, pool *pgxpool.Pool, table string) (bool, error) {
	var exists bool
	query := `
		SELECT EXISTS (
			SELECT 1 
			FROM information_schema.tables 
			WHERE table_schema = 'public' AND table_name = $1
		)
	`
	err := pool.QueryRow(ctx, query, table).Scan(&exists)
	return exists, err
}

func columnExists(ctx context.Context, pool *pgxpool.Pool, table, column string) (bool, string, error) {
	var dataType string
	query := `
		SELECT data_type 
		FROM information_schema.columns 
		WHERE table_schema = 'public' AND table_name = $1 AND column_name = $2
	`
	err := pool.QueryRow(ctx, query, table, column).Scan(&dataType)
	if err != nil {
		return false, "", nil
	}
	return true, dataType, nil
}

func constraintExists(ctx context.Context, pool *pgxpool.Pool, table, constraintName string) (bool, error) {
	var exists bool
	query := `
		SELECT EXISTS (
			SELECT 1 
			FROM pg_constraint c
			JOIN pg_class t ON c.conrelid = t.oid
			JOIN pg_namespace n ON t.relnamespace = n.oid
			WHERE n.nspname = 'public' AND t.relname = $1 AND c.conname = $2
		)
	`
	err := pool.QueryRow(ctx, query, table, constraintName).Scan(&exists)
	return exists, err
}

func constraintDefinition(ctx context.Context, pool *pgxpool.Pool, table, constraintName string) (string, error) {
	var def string
	query := `
		SELECT pg_get_constraintdef(c.oid)
		FROM pg_constraint c
		JOIN pg_class t ON c.conrelid = t.oid
		JOIN pg_namespace n ON t.relnamespace = n.oid
		WHERE n.nspname = 'public' AND t.relname = $1 AND c.conname = $2
	`
	err := pool.QueryRow(ctx, query, table, constraintName).Scan(&def)
	return def, err
}

func indexExists(ctx context.Context, pool *pgxpool.Pool, table, indexName string) (bool, error) {
	var exists bool
	query := `
		SELECT EXISTS (
			SELECT 1 
			FROM pg_indexes 
			WHERE schemaname = 'public' AND tablename = $1 AND indexname = $2
		)
	`
	err := pool.QueryRow(ctx, query, table, indexName).Scan(&exists)
	return exists, err
}

func TestQuotationFoundationSchema(t *testing.T) {
	pool := getTestPool(t)
	ctx := context.Background()

	t.Run("QuotationsTable_Columns_TargetExist", func(t *testing.T) {
		requiredColumns := []string{
			"id",
			"company_id",
			"client_id",
			"quotation_number",
			"subject",
			"solution_type",
			"commercial_scheme",
			"status",
			"current_revision",
			"active_published_revision_id",
			"public_token",
			"created_by",
			"created_at",
			"updated_at",
			"deleted_at",
		}

		for _, col := range requiredColumns {
			t.Run(fmt.Sprintf("column_%s", col), func(t *testing.T) {
				exists, dataType, err := columnExists(ctx, pool, "quotations", col)
				if err != nil {
					t.Fatalf("query error checking column %s: %v", col, err)
				}
				if !exists {
					t.Errorf("expected column 'quotations.%s' to exist, but not found", col)
				} else {
					t.Logf("quotations.%s exists (%s)", col, dataType)
				}
			})
		}
	})

	t.Run("QuotationsTable_Columns_LegacyMustNotExist", func(t *testing.T) {
		legacyColumns := []string{
			"project_type",
		}

		for _, col := range legacyColumns {
			t.Run(fmt.Sprintf("legacy_%s_removed", col), func(t *testing.T) {
				exists, _, err := columnExists(ctx, pool, "quotations", col)
				if err != nil {
					t.Fatalf("query error checking legacy column %s: %v", col, err)
				}
				if exists {
					t.Errorf("expected legacy column 'quotations.%s' to be removed, but it still exists", col)
				}
			})
		}
	})

	t.Run("QuotationsTable_Constraints_And_Indexes", func(t *testing.T) {
		t.Run("chk_quotations_commercial_scheme", func(t *testing.T) {
			exists, err := constraintExists(ctx, pool, "quotations", "chk_quotations_commercial_scheme")
			if err != nil {
				t.Fatalf("error checking constraint: %v", err)
			}
			if !exists {
				t.Error("expected constraint 'chk_quotations_commercial_scheme' on quotations table")
			}
			def, err := constraintDefinition(ctx, pool, "quotations", "chk_quotations_commercial_scheme")
			if err != nil {
				t.Fatalf("error getting constraint definition: %v", err)
			}
			if !strings.Contains(def, "dedicated_squad") || !strings.Contains(def, "subscription_saas") || !strings.Contains(def, "white_label") {
				t.Errorf("expected constraint to allow 'dedicated_squad', 'subscription_saas', and 'white_label', got: %s", def)
			}
		})

		t.Run("fk_quotations_active_published_revision", func(t *testing.T) {
			exists, err := constraintExists(ctx, pool, "quotations", "fk_quotations_active_published_revision")
			if err != nil {
				t.Fatalf("error checking constraint: %v", err)
			}
			if !exists {
				t.Error("expected FK 'fk_quotations_active_published_revision' on quotations table")
			}
		})

		t.Run("idx_quotations_company_id", func(t *testing.T) {
			exists, err := indexExists(ctx, pool, "quotations", "idx_quotations_company_id")
			if err != nil {
				t.Fatalf("error checking index: %v", err)
			}
			if !exists {
				t.Error("expected index 'idx_quotations_company_id' on quotations table")
			}
		})

		t.Run("idx_quotations_company_status", func(t *testing.T) {
			exists, err := indexExists(ctx, pool, "quotations", "idx_quotations_company_status")
			if err != nil {
				t.Fatalf("error checking index: %v", err)
			}
			if !exists {
				t.Error("expected index 'idx_quotations_company_status' on quotations table")
			}
		})

		t.Run("idx_quotations_public_token", func(t *testing.T) {
			exists, err := indexExists(ctx, pool, "quotations", "idx_quotations_public_token")
			if err != nil {
				t.Fatalf("error checking index: %v", err)
			}
			if !exists {
				t.Error("expected index 'idx_quotations_public_token' on quotations table")
			}
		})
	})

	t.Run("QuotationRevisionsTable_Columns_TargetExist", func(t *testing.T) {
		requiredColumns := []string{
			"id",
			"company_id",
			"quotation_id",
			"revision_number",
			"idempotency_key",
			"publication_status",
			"generation_status",
			"generation_input",
			"budget_maximum",
			"allocation_snapshot",
			"rate_card_snapshot",
			"legal_template_snapshot",
			"content_html",
			"content_version",
			"content_hash",
			"ai_provider",
			"ai_model",
			"skill_snapshot",
			"safety_envelope_version",
			"legal_content_modified",
			"legal_modified_by",
			"legal_modified_at",
			"commercial_content_modified",
			"commercial_modified_by",
			"commercial_modified_at",
			"generated_at",
			"published_at",
			"signer_name",
			"signer_email",
			"signature_data",
			"signed_content_hash",
			"signed_at",
			"signed_ip",
			"signed_user_agent",
			"regenerate_count",
			"last_regenerate_at",
			"created_by",
			"created_at",
			"updated_at",
		}

		for _, col := range requiredColumns {
			t.Run(fmt.Sprintf("column_%s", col), func(t *testing.T) {
				exists, dataType, err := columnExists(ctx, pool, "quotation_revisions", col)
				if err != nil {
					t.Fatalf("query error checking column %s: %v", col, err)
				}
				if !exists {
					t.Errorf("expected column 'quotation_revisions.%s' to exist, but not found", col)
				} else {
					t.Logf("quotation_revisions.%s exists (%s)", col, dataType)
				}
			})
		}
	})

	t.Run("QuotationRevisionsTable_Columns_LegacyMustNotExist", func(t *testing.T) {
		legacyColumns := []string{
			"public_token",
			"quotation_date",
			"valid_until",
			"client_snapshot",
			"project_description",
			"scope_of_work",
			"maintenance_info",
			"hosting_info",
			"notes",
			"signature_image_url",
			"product_detail_data",
			"budget",
		}

		for _, col := range legacyColumns {
			t.Run(fmt.Sprintf("legacy_%s_removed", col), func(t *testing.T) {
				exists, _, err := columnExists(ctx, pool, "quotation_revisions", col)
				if err != nil {
					t.Fatalf("query error checking legacy column %s: %v", col, err)
				}
				if exists {
					t.Errorf("expected legacy column 'quotation_revisions.%s' to be removed, but it still exists", col)
				}
			})
		}
	})

	t.Run("QuotationRevisionsTable_Constraints_And_Indexes", func(t *testing.T) {
		t.Run("uq_revision_number", func(t *testing.T) {
			exists, err := constraintExists(ctx, pool, "quotation_revisions", "uq_revision_number")
			if err != nil {
				t.Fatalf("error checking constraint: %v", err)
			}
			if !exists {
				t.Error("expected UNIQUE constraint 'uq_revision_number' on quotation_revisions table")
			}
		})

		t.Run("uq_revisions_idempotency_key", func(t *testing.T) {
			exists, err := indexExists(ctx, pool, "quotation_revisions", "uq_revisions_idempotency_key")
			if err != nil {
				t.Fatalf("error checking index: %v", err)
			}
			if !exists {
				t.Error("expected unique index 'uq_revisions_idempotency_key' on quotation_revisions table")
			}
		})

		t.Run("uq_revisions_one_draft_per_quotation", func(t *testing.T) {
			exists, err := indexExists(ctx, pool, "quotation_revisions", "uq_revisions_one_draft_per_quotation")
			if err != nil {
				t.Fatalf("error checking index: %v", err)
			}
			if !exists {
				t.Error("expected unique index 'uq_revisions_one_draft_per_quotation' on quotation_revisions table")
			}
		})

		t.Run("uq_revisions_one_published_per_quotation", func(t *testing.T) {
			exists, err := indexExists(ctx, pool, "quotation_revisions", "uq_revisions_one_published_per_quotation")
			if err != nil {
				t.Fatalf("error checking index: %v", err)
			}
			if !exists {
				t.Error("expected unique index 'uq_revisions_one_published_per_quotation' on quotation_revisions table")
			}
		})

		t.Run("chk_budget_maximum_non_negative", func(t *testing.T) {
			exists, err := constraintExists(ctx, pool, "quotation_revisions", "chk_budget_maximum_non_negative")
			if err != nil {
				t.Fatalf("error checking constraint: %v", err)
			}
			if !exists {
				t.Error("expected constraint 'chk_budget_maximum_non_negative' on quotation_revisions table")
			}
		})

		t.Run("chk_content_version_positive", func(t *testing.T) {
			exists, err := constraintExists(ctx, pool, "quotation_revisions", "chk_content_version_positive")
			if err != nil {
				t.Fatalf("error checking constraint: %v", err)
			}
			if !exists {
				t.Error("expected constraint 'chk_content_version_positive' on quotation_revisions table")
			}
		})

		t.Run("chk_regenerate_count_non_negative", func(t *testing.T) {
			exists, err := constraintExists(ctx, pool, "quotation_revisions", "chk_regenerate_count_non_negative")
			if err != nil {
				t.Fatalf("error checking constraint: %v", err)
			}
			if !exists {
				t.Error("expected constraint 'chk_regenerate_count_non_negative' on quotation_revisions table")
			}
		})
	})
}

func TestBlueprintFoundationSchema(t *testing.T) {
	pool := getTestPool(t)
	ctx := context.Background()

	t.Run("BlueprintsTable_Columns", func(t *testing.T) {
		requiredColumns := []string{
			"id",
			"company_id",
			"quotation_id",
			"title",
			"status",
			"current_revision",
			"active_published_revision_id",
			"created_by",
			"created_at",
			"updated_at",
			"deleted_at",
		}

		for _, col := range requiredColumns {
			t.Run(fmt.Sprintf("column_%s", col), func(t *testing.T) {
				exists, dataType, err := columnExists(ctx, pool, "blueprints", col)
				if err != nil {
					t.Fatalf("query error: %v", err)
				}
				if !exists {
					t.Errorf("expected column 'blueprints.%s' to exist, but not found", col)
				} else {
					t.Logf("blueprints.%s exists (%s)", col, dataType)
				}
			})
		}
	})

	t.Run("BlueprintsTable_Constraints_And_Indexes", func(t *testing.T) {
		t.Run("unique_quotation_id", func(t *testing.T) {
			exists, err := constraintExists(ctx, pool, "blueprints", "blueprints_quotation_id_key")
			if err != nil {
				t.Fatalf("query error: %v", err)
			}
			if !exists {
				// Also check unique index if constraint name differs
				existsIdx, err := indexExists(ctx, pool, "blueprints", "blueprints_quotation_id_key")
				if err != nil || !existsIdx {
					t.Error("expected unique constraint or index on blueprints(quotation_id)")
				}
			}
		})

		t.Run("fk_blueprints_active_published_revision", func(t *testing.T) {
			exists, err := constraintExists(ctx, pool, "blueprints", "fk_blueprints_active_published_revision")
			if err != nil {
				t.Fatalf("query error: %v", err)
			}
			if !exists {
				t.Error("expected FK 'fk_blueprints_active_published_revision' on blueprints table")
			}
		})

		t.Run("idx_blueprints_company", func(t *testing.T) {
			exists, err := indexExists(ctx, pool, "blueprints", "idx_blueprints_company")
			if err != nil {
				t.Fatalf("query error: %v", err)
			}
			if !exists {
				t.Error("expected index 'idx_blueprints_company' on blueprints table")
			}
		})

		t.Run("idx_blueprints_quotation", func(t *testing.T) {
			exists, err := indexExists(ctx, pool, "blueprints", "idx_blueprints_quotation")
			if err != nil {
				t.Fatalf("query error: %v", err)
			}
			if !exists {
				t.Error("expected index 'idx_blueprints_quotation' on blueprints table")
			}
		})
	})

	t.Run("BlueprintRevisionsTable_Columns", func(t *testing.T) {
		requiredColumns := []string{
			"id",
			"company_id",
			"blueprint_id",
			"revision_number",
			"idempotency_key",
			"source_quotation_revision_id",
			"source_context_snapshot",
			"publication_status",
			"completeness_status",
			"generation_status",
			"content_json",
			"content_version",
			"validation_result",
			"cross_validation_result",
			"skill_snapshot",
			"safety_envelope_version",
			"ai_provider",
			"ai_model",
			"generated_at",
			"published_at",
			"created_by",
			"created_at",
			"updated_at",
		}

		for _, col := range requiredColumns {
			t.Run(fmt.Sprintf("column_%s", col), func(t *testing.T) {
				exists, dataType, err := columnExists(ctx, pool, "blueprint_revisions", col)
				if err != nil {
					t.Fatalf("query error: %v", err)
				}
				if !exists {
					t.Errorf("expected column 'blueprint_revisions.%s' to exist, but not found", col)
				} else {
					t.Logf("blueprint_revisions.%s exists (%s)", col, dataType)
				}
			})
		}
	})

	t.Run("BlueprintRevisionsTable_Constraints_And_Indexes", func(t *testing.T) {
		t.Run("uq_blueprint_revision_number", func(t *testing.T) {
			exists, err := constraintExists(ctx, pool, "blueprint_revisions", "uq_blueprint_revision_number")
			if err != nil {
				t.Fatalf("query error: %v", err)
			}
			if !exists {
				t.Error("expected constraint 'uq_blueprint_revision_number' on blueprint_revisions")
			}
		})

		t.Run("uq_blueprint_revisions_one_draft", func(t *testing.T) {
			exists, err := indexExists(ctx, pool, "blueprint_revisions", "uq_blueprint_revisions_one_draft")
			if err != nil {
				t.Fatalf("query error: %v", err)
			}
			if !exists {
				t.Error("expected unique index 'uq_blueprint_revisions_one_draft' on blueprint_revisions")
			}
		})

		t.Run("uq_blueprint_revisions_one_published", func(t *testing.T) {
			exists, err := indexExists(ctx, pool, "blueprint_revisions", "uq_blueprint_revisions_one_published")
			if err != nil {
				t.Fatalf("query error: %v", err)
			}
			if !exists {
				t.Error("expected unique index 'uq_blueprint_revisions_one_published' on blueprint_revisions")
			}
		})

		t.Run("uq_blueprint_revisions_idempotency_key", func(t *testing.T) {
			exists, err := indexExists(ctx, pool, "blueprint_revisions", "uq_blueprint_revisions_idempotency_key")
			if err != nil {
				t.Fatalf("query error: %v", err)
			}
			if !exists {
				t.Error("expected unique index 'uq_blueprint_revisions_idempotency_key' on blueprint_revisions")
			}
		})

		t.Run("chk_bp_completeness_status", func(t *testing.T) {
			exists, err := constraintExists(ctx, pool, "blueprint_revisions", "chk_bp_completeness_status")
			if err != nil {
				t.Fatalf("query error: %v", err)
			}
			if !exists {
				t.Error("expected constraint 'chk_bp_completeness_status' on blueprint_revisions")
			}
		})

		t.Run("chk_bp_content_version_positive", func(t *testing.T) {
			exists, err := constraintExists(ctx, pool, "blueprint_revisions", "chk_bp_content_version_positive")
			if err != nil {
				t.Fatalf("query error: %v", err)
			}
			if !exists {
				t.Error("expected constraint 'chk_bp_content_version_positive' on blueprint_revisions")
			}
		})
	})

	t.Run("BlueprintSectionRunsTable_Columns", func(t *testing.T) {
		requiredColumns := []string{
			"id",
			"company_id",
			"blueprint_revision_id",
			"section_type",
			"status",
			"attempt_count",
			"content_json",
			"validation_result",
			"safe_error_code",
			"started_at",
			"completed_at",
			"updated_at",
		}

		for _, col := range requiredColumns {
			t.Run(fmt.Sprintf("column_%s", col), func(t *testing.T) {
				exists, dataType, err := columnExists(ctx, pool, "blueprint_section_runs", col)
				if err != nil {
					t.Fatalf("query error: %v", err)
				}
				if !exists {
					t.Errorf("expected column 'blueprint_section_runs.%s' to exist, but not found", col)
				} else {
					t.Logf("blueprint_section_runs.%s exists (%s)", col, dataType)
				}
			})
		}
	})

	t.Run("BlueprintSectionRunsTable_Constraints_And_Indexes", func(t *testing.T) {
		t.Run("chk_section_type", func(t *testing.T) {
			exists, err := constraintExists(ctx, pool, "blueprint_section_runs", "chk_section_type")
			if err != nil {
				t.Fatalf("query error: %v", err)
			}
			if !exists {
				t.Error("expected constraint 'chk_section_type' on blueprint_section_runs")
			}
		})

		t.Run("uq_section_run_per_revision", func(t *testing.T) {
			exists, err := constraintExists(ctx, pool, "blueprint_section_runs", "uq_section_run_per_revision")
			if err != nil {
				t.Fatalf("query error: %v", err)
			}
			if !exists {
				t.Error("expected constraint 'uq_section_run_per_revision' on blueprint_section_runs")
			}
		})

		t.Run("idx_section_runs_revision", func(t *testing.T) {
			exists, err := indexExists(ctx, pool, "blueprint_section_runs", "idx_section_runs_revision")
			if err != nil {
				t.Fatalf("query error: %v", err)
			}
			if !exists {
				t.Error("expected index 'idx_section_runs_revision' on blueprint_section_runs")
			}
		})
	})
}

func TestSupportTablesSchema(t *testing.T) {
	pool := getTestPool(t)
	ctx := context.Background()

	t.Run("ai_skills", func(t *testing.T) {
		requiredColumns := []string{
			"id", "company_id", "name", "command", "content_instruction",
			"is_active", "created_by", "created_at", "updated_at", "deleted_at",
		}
		for _, col := range requiredColumns {
			exists, _, err := columnExists(ctx, pool, "ai_skills", col)
			if err != nil || !exists {
				t.Errorf("expected column 'ai_skills.%s' to exist", col)
			}
		}

		exists, err := indexExists(ctx, pool, "ai_skills", "uq_skills_active_command")
		if err != nil || !exists {
			t.Error("expected unique index 'uq_skills_active_command' on ai_skills")
		}

		exists, err = indexExists(ctx, pool, "ai_skills", "idx_skills_company")
		if err != nil || !exists {
			t.Error("expected index 'idx_skills_company' on ai_skills")
		}
	})

	t.Run("ai_credentials", func(t *testing.T) {
		requiredColumns := []string{
			"id", "company_id", "provider", "model", "encrypted_api_key",
			"key_hint", "is_active", "created_by", "created_at", "updated_at",
		}
		for _, col := range requiredColumns {
			exists, _, err := columnExists(ctx, pool, "ai_credentials", col)
			if err != nil || !exists {
				t.Errorf("expected column 'ai_credentials.%s' to exist", col)
			}
		}

		exists, err := indexExists(ctx, pool, "ai_credentials", "uq_credentials_active_per_company")
		if err != nil || !exists {
			t.Error("expected unique index 'uq_credentials_active_per_company' on ai_credentials")
		}

		exists, err = indexExists(ctx, pool, "ai_credentials", "idx_credentials_company")
		if err != nil || !exists {
			t.Error("expected index 'idx_credentials_company' on ai_credentials")
		}
	})

	t.Run("rate_cards", func(t *testing.T) {
		requiredColumns := []string{
			"id", "company_id", "code", "name", "market", "currency",
			"effective_from", "effective_until", "is_active", "created_by",
			"created_at", "updated_at",
		}
		for _, col := range requiredColumns {
			exists, _, err := columnExists(ctx, pool, "rate_cards", col)
			if err != nil || !exists {
				t.Errorf("expected column 'rate_cards.%s' to exist", col)
			}
		}

		exists, err := constraintExists(ctx, pool, "rate_cards", "chk_rate_cards_market")
		if err != nil || !exists {
			t.Error("expected constraint 'chk_rate_cards_market' on rate_cards")
		}

		exists, err = constraintExists(ctx, pool, "rate_cards", "uq_rate_cards_company_code")
		if err != nil || !exists {
			t.Error("expected constraint 'uq_rate_cards_company_code' on rate_cards")
		}

		exists, err = indexExists(ctx, pool, "rate_cards", "idx_rate_cards_company")
		if err != nil || !exists {
			t.Error("expected index 'idx_rate_cards_company' on rate_cards")
		}
	})

	t.Run("rate_card_items", func(t *testing.T) {
		requiredColumns := []string{
			"id", "company_id", "rate_card_id", "role", "seniority",
			"term_type", "amount_per_month", "created_at", "updated_at",
		}
		for _, col := range requiredColumns {
			exists, _, err := columnExists(ctx, pool, "rate_card_items", col)
			if err != nil || !exists {
				t.Errorf("expected column 'rate_card_items.%s' to exist", col)
			}
		}

		exists, err := constraintExists(ctx, pool, "rate_card_items", "chk_rate_card_items_term_type")
		if err != nil || !exists {
			t.Error("expected constraint 'chk_rate_card_items_term_type' on rate_card_items")
		}

		exists, err = constraintExists(ctx, pool, "rate_card_items", "chk_rate_card_items_amount_positive")
		if err != nil || !exists {
			t.Error("expected constraint 'chk_rate_card_items_amount_positive' on rate_card_items")
		}

		exists, err = constraintExists(ctx, pool, "rate_card_items", "uq_rate_card_items_unique")
		if err != nil || !exists {
			t.Error("expected constraint 'uq_rate_card_items_unique' on rate_card_items")
		}

		exists, err = indexExists(ctx, pool, "rate_card_items", "idx_rate_card_items_rate_card")
		if err != nil || !exists {
			t.Error("expected index 'idx_rate_card_items_rate_card' on rate_card_items")
		}
	})

	t.Run("legal_templates", func(t *testing.T) {
		requiredColumns := []string{
			"id", "company_id", "code", "version", "content_html",
			"is_active", "approved_by", "approved_at", "created_at",
		}
		for _, col := range requiredColumns {
			exists, _, err := columnExists(ctx, pool, "legal_templates", col)
			if err != nil || !exists {
				t.Errorf("expected column 'legal_templates.%s' to exist", col)
			}
		}

		exists, err := constraintExists(ctx, pool, "legal_templates", "uq_legal_templates_versioned")
		if err != nil || !exists {
			t.Error("expected constraint 'uq_legal_templates_versioned' on legal_templates")
		}

		exists, err = indexExists(ctx, pool, "legal_templates", "idx_legal_templates_company")
		if err != nil || !exists {
			t.Error("expected index 'idx_legal_templates_company' on legal_templates")
		}
	})

	t.Run("blueprint_share_links", func(t *testing.T) {
		requiredColumns := []string{
			"id", "company_id", "blueprint_id", "blueprint_revision_id",
			"token_hash", "label", "created_by", "created_at", "revoked_at",
			"last_accessed_at",
		}
		for _, col := range requiredColumns {
			exists, _, err := columnExists(ctx, pool, "blueprint_share_links", col)
			if err != nil || !exists {
				t.Errorf("expected column 'blueprint_share_links.%s' to exist", col)
			}
		}

		exists, err := constraintExists(ctx, pool, "blueprint_share_links", "blueprint_share_links_token_hash_key")
		if err != nil || !exists {
			// Check unique index as fallback
			existsIdx, err := indexExists(ctx, pool, "blueprint_share_links", "blueprint_share_links_token_hash_key")
			if err != nil || !existsIdx {
				t.Error("expected UNIQUE constraint on blueprint_share_links(token_hash)")
			}
		}

		exists, err = indexExists(ctx, pool, "blueprint_share_links", "idx_share_links_company")
		if err != nil || !exists {
			t.Error("expected index 'idx_share_links_company' on blueprint_share_links")
		}

		exists, err = indexExists(ctx, pool, "blueprint_share_links", "idx_share_links_blueprint")
		if err != nil || !exists {
			t.Error("expected index 'idx_share_links_blueprint' on blueprint_share_links")
		}

		exists, err = indexExists(ctx, pool, "blueprint_share_links", "idx_share_links_token_hash")
		if err != nil || !exists {
			t.Error("expected index 'idx_share_links_token_hash' on blueprint_share_links")
		}
	})
}

func TestConstraintViolations(t *testing.T) {
	pool := getTestPool(t)
	ctx := context.Background()

	// Check if blueprints table exists before running DML tests
	exists, err := tableExists(ctx, pool, "blueprints")
	if err != nil || !exists {
		t.Skip("skipping constraint violation tests because blueprints table does not exist yet")
	}

	t.Run("DuplicateBlueprintPerQuotation", func(t *testing.T) {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("failed to start tx: %v", err)
		}
		defer tx.Rollback(ctx)

		// 1. Insert quotation
		var quotID string
		err = tx.QueryRow(ctx, `
			INSERT INTO quotations (company_id, client_id, quotation_number, solution_type, subject)
			VALUES (gen_random_uuid(), gen_random_uuid(), 'QUO-TEST-DUP-BP', 'custom_software', 'Test Duplicate BP')
			RETURNING id
		`).Scan(&quotID)
		if err != nil {
			t.Fatalf("failed to insert quotation: %v", err)
		}

		// 2. Insert first blueprint
		var bp1ID string
		err = tx.QueryRow(ctx, `
			INSERT INTO blueprints (company_id, quotation_id, title)
			VALUES (gen_random_uuid(), $1, 'BP 1')
			RETURNING id
		`, quotID).Scan(&bp1ID)
		if err != nil {
			t.Fatalf("failed to insert first blueprint: %v", err)
		}

		// 3. Insert second blueprint with same quotation_id -> MUST FAIL
		_, err = tx.Exec(ctx, `
			INSERT INTO blueprints (company_id, quotation_id, title)
			VALUES (gen_random_uuid(), $1, 'BP 2')
		`, quotID)
		if err == nil {
			t.Error("expected error inserting duplicate blueprint for same quotation, but got none")
		} else {
			t.Logf("correctly rejected duplicate blueprint: %v", err)
		}
	})

	t.Run("DuplicateRevisionNumberPerBlueprint", func(t *testing.T) {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("failed to start tx: %v", err)
		}
		defer tx.Rollback(ctx)

		// 1. Insert quotation + quotation revision
		var quotID, qRevID, bpID string
		companyID := "00000000-0000-0000-0000-000000000001"
		err = tx.QueryRow(ctx, `
			INSERT INTO quotations (company_id, client_id, quotation_number, solution_type, subject)
			VALUES ($1, gen_random_uuid(), 'QUO-TEST-DUP-REV', 'custom_software', 'Test Duplicate Rev')
			RETURNING id
		`, companyID).Scan(&quotID)
		if err != nil {
			t.Fatalf("failed to insert quotation: %v", err)
		}

		err = tx.QueryRow(ctx, `
			INSERT INTO quotation_revisions (company_id, quotation_id, revision_number, budget_maximum)
			VALUES ($1, $2, 1, 1000000)
			RETURNING id
		`, companyID, quotID).Scan(&qRevID)
		if err != nil {
			t.Fatalf("failed to insert quotation revision: %v", err)
		}

		err = tx.QueryRow(ctx, `
			INSERT INTO blueprints (company_id, quotation_id, title)
			VALUES ($1, $2, 'BP Test Rev')
			RETURNING id
		`, companyID, quotID).Scan(&bpID)
		if err != nil {
			t.Fatalf("failed to insert blueprint: %v", err)
		}

		// 2. Insert blueprint revision 1
		_, err = tx.Exec(ctx, `
			INSERT INTO blueprint_revisions (company_id, blueprint_id, revision_number, source_quotation_revision_id, source_context_snapshot)
			VALUES ($1, $2, 1, $3, '{}')
		`, companyID, bpID, qRevID)
		if err != nil {
			t.Fatalf("failed to insert blueprint revision 1: %v", err)
		}

		// 3. Insert blueprint revision 1 again -> MUST FAIL
		_, err = tx.Exec(ctx, `
			INSERT INTO blueprint_revisions (company_id, blueprint_id, revision_number, source_quotation_revision_id, source_context_snapshot)
			VALUES ($1, $2, 1, $3, '{}')
		`, companyID, bpID, qRevID)
		if err == nil {
			t.Error("expected error inserting duplicate revision number for blueprint, but got none")
		} else {
			t.Logf("correctly rejected duplicate revision number: %v", err)
		}
	})

	t.Run("DuplicateSectionTypePerRevision", func(t *testing.T) {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("failed to start tx: %v", err)
		}
		defer tx.Rollback(ctx)

		var quotID, qRevID, bpID, bpRevID string
		companyID := "00000000-0000-0000-0000-000000000001"
		err = tx.QueryRow(ctx, `
			INSERT INTO quotations (company_id, client_id, quotation_number, solution_type, subject)
			VALUES ($1, gen_random_uuid(), 'QUO-TEST-DUP-SEC', 'custom_software', 'Test Duplicate Section')
			RETURNING id
		`, companyID).Scan(&quotID)
		if err != nil {
			t.Fatalf("failed to insert quotation: %v", err)
		}

		err = tx.QueryRow(ctx, `
			INSERT INTO quotation_revisions (company_id, quotation_id, revision_number, budget_maximum)
			VALUES ($1, $2, 1, 1000000)
			RETURNING id
		`, companyID, quotID).Scan(&qRevID)
		if err != nil {
			t.Fatalf("failed to insert quotation revision: %v", err)
		}

		err = tx.QueryRow(ctx, `
			INSERT INTO blueprints (company_id, quotation_id, title)
			VALUES ($1, $2, 'BP Test Section')
			RETURNING id
		`, companyID, quotID).Scan(&bpID)
		if err != nil {
			t.Fatalf("failed to insert blueprint: %v", err)
		}

		err = tx.QueryRow(ctx, `
			INSERT INTO blueprint_revisions (company_id, blueprint_id, revision_number, source_quotation_revision_id, source_context_snapshot)
			VALUES ($1, $2, 1, $3, '{}')
			RETURNING id
		`, companyID, bpID, qRevID).Scan(&bpRevID)
		if err != nil {
			t.Fatalf("failed to insert blueprint revision: %v", err)
		}

		// 1. Insert section_run for 'journey'
		_, err = tx.Exec(ctx, `
			INSERT INTO blueprint_section_runs (company_id, blueprint_revision_id, section_type)
			VALUES ($1, $2, 'journey')
		`, companyID, bpRevID)
		if err != nil {
			t.Fatalf("failed to insert first section run: %v", err)
		}

		// 2. Insert section_run for 'journey' again -> MUST FAIL
		_, err = tx.Exec(ctx, `
			INSERT INTO blueprint_section_runs (company_id, blueprint_revision_id, section_type)
			VALUES ($1, $2, 'journey')
		`, companyID, bpRevID)
		if err == nil {
			t.Error("expected error inserting duplicate section type for same revision, but got none")
		} else {
			t.Logf("correctly rejected duplicate section run: %v", err)
		}
	})

	t.Run("DuplicateRateCardItemCombo", func(t *testing.T) {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("failed to start tx: %v", err)
		}
		defer tx.Rollback(ctx)

		var cardID string
		companyID := "00000000-0000-0000-0000-000000000001"
		err = tx.QueryRow(ctx, `
			INSERT INTO rate_cards (company_id, code, name, market, effective_from)
			VALUES ($1, 'RC-TEST-DUP', 'Rate Card Dup Test', 'national', CURRENT_DATE)
			RETURNING id
		`, companyID).Scan(&cardID)
		if err != nil {
			t.Fatalf("failed to insert rate card: %v", err)
		}

		// 1. Insert rate card item
		_, err = tx.Exec(ctx, `
			INSERT INTO rate_card_items (company_id, rate_card_id, role, seniority, term_type, amount_per_month)
			VALUES ($1, $2, 'software_engineer', 'senior', 'short_term', 25000000)
		`, companyID, cardID)
		if err != nil {
			t.Fatalf("failed to insert first rate card item: %v", err)
		}

		// 2. Insert same combo -> MUST FAIL
		_, err = tx.Exec(ctx, `
			INSERT INTO rate_card_items (company_id, rate_card_id, role, seniority, term_type, amount_per_month)
			VALUES ($1, $2, 'software_engineer', 'senior', 'short_term', 30000000)
		`, companyID, cardID)
		if err == nil {
			t.Error("expected error inserting duplicate rate card item combo, but got none")
		} else {
			t.Logf("correctly rejected duplicate rate card item combo: %v", err)
		}
	})

	t.Run("InvalidSectionType", func(t *testing.T) {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("failed to start tx: %v", err)
		}
		defer tx.Rollback(ctx)

		var quotID, qRevID, bpID, bpRevID string
		companyID := "00000000-0000-0000-0000-000000000001"
		err = tx.QueryRow(ctx, `
			INSERT INTO quotations (company_id, client_id, quotation_number, solution_type, subject)
			VALUES ($1, gen_random_uuid(), 'QUO-TEST-INV-SEC', 'custom_software', 'Test Invalid Section')
			RETURNING id
		`, companyID).Scan(&quotID)
		if err != nil {
			t.Fatalf("failed to insert quotation: %v", err)
		}

		err = tx.QueryRow(ctx, `
			INSERT INTO quotation_revisions (company_id, quotation_id, revision_number, budget_maximum)
			VALUES ($1, $2, 1, 1000000)
			RETURNING id
		`, companyID, quotID).Scan(&qRevID)
		if err != nil {
			t.Fatalf("failed to insert quotation revision: %v", err)
		}

		err = tx.QueryRow(ctx, `
			INSERT INTO blueprints (company_id, quotation_id, title)
			VALUES ($1, $2, 'BP Test Invalid Sec')
			RETURNING id
		`, companyID, quotID).Scan(&bpID)
		if err != nil {
			t.Fatalf("failed to insert blueprint: %v", err)
		}

		err = tx.QueryRow(ctx, `
			INSERT INTO blueprint_revisions (company_id, blueprint_id, revision_number, source_quotation_revision_id, source_context_snapshot)
			VALUES ($1, $2, 1, $3, '{}')
			RETURNING id
		`, companyID, bpID, qRevID).Scan(&bpRevID)
		if err != nil {
			t.Fatalf("failed to insert blueprint revision: %v", err)
		}

		// Insert section_run with invalid section_type -> MUST FAIL
		_, err = tx.Exec(ctx, `
			INSERT INTO blueprint_section_runs (company_id, blueprint_revision_id, section_type)
			VALUES ($1, $2, 'non_existent_section_type')
		`, companyID, bpRevID)
		if err == nil {
			t.Error("expected error on invalid section_type check constraint, but got none")
		} else {
			t.Logf("correctly rejected invalid section type: %v", err)
		}
	})

	t.Run("RateCardItemAmountZero", func(t *testing.T) {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("failed to start tx: %v", err)
		}
		defer tx.Rollback(ctx)

		var cardID string
		companyID := "00000000-0000-0000-0000-000000000001"
		err = tx.QueryRow(ctx, `
			INSERT INTO rate_cards (company_id, code, name, market, effective_from)
			VALUES ($1, 'RC-TEST-ZERO', 'Rate Card Zero Test', 'national', CURRENT_DATE)
			RETURNING id
		`, companyID).Scan(&cardID)
		if err != nil {
			t.Fatalf("failed to insert rate card: %v", err)
		}

		// Insert rate card item with amount_per_month = 0 -> MUST FAIL
		_, err = tx.Exec(ctx, `
			INSERT INTO rate_card_items (company_id, rate_card_id, role, seniority, term_type, amount_per_month)
			VALUES ($1, $2, 'software_engineer', 'senior', 'short_term', 0)
		`, companyID, cardID)
		if err == nil {
			t.Error("expected error on amount_per_month > 0 check constraint, but got none")
		} else {
			t.Logf("correctly rejected zero amount rate card item: %v", err)
		}
	})

	t.Run("CompletenessStatusInvalid", func(t *testing.T) {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("failed to start tx: %v", err)
		}
		defer tx.Rollback(ctx)

		var quotID, qRevID, bpID string
		companyID := "00000000-0000-0000-0000-000000000001"
		err = tx.QueryRow(ctx, `
			INSERT INTO quotations (company_id, client_id, quotation_number, solution_type, subject)
			VALUES ($1, gen_random_uuid(), 'QUO-TEST-INV-COMPL', 'custom_software', 'Test Invalid Completeness')
			RETURNING id
		`, companyID).Scan(&quotID)
		if err != nil {
			t.Fatalf("failed to insert quotation: %v", err)
		}

		err = tx.QueryRow(ctx, `
			INSERT INTO quotation_revisions (company_id, quotation_id, revision_number, budget_maximum)
			VALUES ($1, $2, 1, 1000000)
			RETURNING id
		`, companyID, quotID).Scan(&qRevID)
		if err != nil {
			t.Fatalf("failed to insert quotation revision: %v", err)
		}

		err = tx.QueryRow(ctx, `
			INSERT INTO blueprints (company_id, quotation_id, title)
			VALUES ($1, $2, 'BP Test Invalid Compl')
			RETURNING id
		`, companyID, quotID).Scan(&bpID)
		if err != nil {
			t.Fatalf("failed to insert blueprint: %v", err)
		}

		// Insert blueprint_revision with invalid completeness_status -> MUST FAIL
		_, err = tx.Exec(ctx, `
			INSERT INTO blueprint_revisions (company_id, blueprint_id, revision_number, source_quotation_revision_id, source_context_snapshot, completeness_status)
			VALUES ($1, $2, 1, $3, '{}', 'invalid_completeness')
		`, companyID, bpID, qRevID)
		if err == nil {
			t.Error("expected error on completeness_status check constraint, but got none")
		} else {
			t.Logf("correctly rejected invalid completeness_status: %v", err)
		}
	})
}

func TestCommercialSchemeConstraint(t *testing.T) {
	pool := getTestPool(t)
	ctx := context.Background()

	t.Run("AcceptsSubscriptionSaaS", func(t *testing.T) {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("failed to begin tx: %v", err)
		}
		defer tx.Rollback(ctx)

		var id string
		err = tx.QueryRow(ctx, `
			INSERT INTO quotations (company_id, client_id, quotation_number, solution_type, commercial_scheme, subject)
			VALUES (gen_random_uuid(), gen_random_uuid(), 'QUO-TEST-SAAS', 'saas', 'subscription_saas', 'Test SaaS Scheme')
			RETURNING id
		`).Scan(&id)
		if err != nil {
			t.Fatalf("expected insert with subscription_saas to succeed, but failed: %v", err)
		}
		if id == "" {
			t.Errorf("expected returning id, got empty")
		}
	})

	t.Run("AcceptsDedicatedSquad", func(t *testing.T) {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("failed to begin tx: %v", err)
		}
		defer tx.Rollback(ctx)

		var id string
		err = tx.QueryRow(ctx, `
			INSERT INTO quotations (company_id, client_id, quotation_number, solution_type, commercial_scheme, subject)
			VALUES (gen_random_uuid(), gen_random_uuid(), 'QUO-TEST-SQUAD', 'custom_software', 'dedicated_squad', 'Test Squad Scheme')
			RETURNING id
		`).Scan(&id)
		if err != nil {
			t.Fatalf("expected insert with dedicated_squad to succeed, but failed: %v", err)
		}
		if id == "" {
			t.Errorf("expected returning id, got empty")
		}
	})

	t.Run("AcceptsWhiteLabel", func(t *testing.T) {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("failed to begin tx: %v", err)
		}
		defer tx.Rollback(ctx)

		var id string
		err = tx.QueryRow(ctx, `
			INSERT INTO quotations (company_id, client_id, quotation_number, solution_type, commercial_scheme, subject)
			VALUES (gen_random_uuid(), gen_random_uuid(), 'QUO-TEST-WL', 'custom_software', 'white_label', 'Test White Label Scheme')
			RETURNING id
		`).Scan(&id)
		if err != nil {
			t.Fatalf("expected insert with white_label to succeed, but failed: %v", err)
		}
		if id == "" {
			t.Errorf("expected returning id, got empty")
		}
	})

	t.Run("RejectsUnknownCommercialScheme", func(t *testing.T) {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("failed to begin tx: %v", err)
		}
		defer tx.Rollback(ctx)

		_, err = tx.Exec(ctx, `
			INSERT INTO quotations (company_id, client_id, quotation_number, solution_type, commercial_scheme, subject)
			VALUES (gen_random_uuid(), gen_random_uuid(), 'QUO-TEST-UNKNOWN', 'custom_software', 'unknown_scheme', 'Test Unknown Scheme')
		`)
		if err == nil {
			t.Error("expected check constraint error for unknown commercial scheme, but got none")
		} else {
			t.Logf("correctly rejected unknown commercial scheme: %v", err)
		}
	})
}

func TestQuotationCommercialSchemeConstraintAllowsWhiteLabel(t *testing.T) {
	pool := testDB(t)
	ctx := context.Background()
	var definition string
	err := pool.QueryRow(ctx, `
		SELECT pg_get_constraintdef(oid)
		FROM pg_constraint
		WHERE conname = 'chk_quotations_commercial_scheme'
	`).Scan(&definition)
	require.NoError(t, err)
	assert.Contains(t, definition, "dedicated_squad")
	assert.Contains(t, definition, "subscription_saas")
	assert.Contains(t, definition, "white_label")
}
