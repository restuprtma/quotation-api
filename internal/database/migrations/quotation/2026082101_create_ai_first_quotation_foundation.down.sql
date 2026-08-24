-- ============================================================
-- Down: Revert AI-First Quotation Foundation
-- ============================================================

-- ── 9. Drop FK active_published_revision_id ──────────────────
ALTER TABLE quotations
    DROP CONSTRAINT IF EXISTS fk_quotations_active_published_revision;

-- ── 8. Drop revision constraints & indexes ───────────────────
ALTER TABLE quotation_revisions
    DROP CONSTRAINT IF EXISTS chk_content_version_positive;

ALTER TABLE quotation_revisions
    DROP CONSTRAINT IF EXISTS chk_budget_maximum_non_negative;

DROP INDEX IF EXISTS uq_revisions_one_published_per_quotation;
DROP INDEX IF EXISTS uq_revisions_one_draft_per_quotation;
DROP INDEX IF EXISTS uq_revisions_idempotency_key;

ALTER TABLE quotation_revisions
    DROP CONSTRAINT IF EXISTS uq_revision_number;

-- ── 7. Drop new revision columns ────────────────────────────
ALTER TABLE quotation_revisions
    DROP COLUMN IF EXISTS idempotency_key,
    DROP COLUMN IF EXISTS publication_status,
    DROP COLUMN IF EXISTS generation_status,
    DROP COLUMN IF EXISTS generation_input,
    DROP COLUMN IF EXISTS allocation_snapshot,
    DROP COLUMN IF EXISTS rate_card_snapshot,
    DROP COLUMN IF EXISTS legal_template_snapshot,
    DROP COLUMN IF EXISTS content_html,
    DROP COLUMN IF EXISTS content_version,
    DROP COLUMN IF EXISTS content_hash,
    DROP COLUMN IF EXISTS ai_provider,
    DROP COLUMN IF EXISTS ai_model,
    DROP COLUMN IF EXISTS skill_snapshot,
    DROP COLUMN IF EXISTS safety_envelope_version,
    DROP COLUMN IF EXISTS legal_content_modified,
    DROP COLUMN IF EXISTS legal_modified_by,
    DROP COLUMN IF EXISTS legal_modified_at,
    DROP COLUMN IF EXISTS commercial_content_modified,
    DROP COLUMN IF EXISTS commercial_modified_by,
    DROP COLUMN IF EXISTS commercial_modified_at,
    DROP COLUMN IF EXISTS generated_at,
    DROP COLUMN IF EXISTS published_at,
    DROP COLUMN IF EXISTS signer_name,
    DROP COLUMN IF EXISTS signer_email,
    DROP COLUMN IF EXISTS signature_data,
    DROP COLUMN IF EXISTS signed_content_hash,
    DROP COLUMN IF EXISTS signed_user_agent;

-- ── 6. Rename budget_maximum → budget ────────────────────────
ALTER TABLE quotation_revisions RENAME COLUMN budget_maximum TO budget;

-- ── 5. Re-add legacy revision columns ────────────────────────
ALTER TABLE quotation_revisions
    ADD COLUMN public_token UUID UNIQUE NOT NULL DEFAULT gen_random_uuid(),
    ADD COLUMN quotation_date DATE,
    ADD COLUMN valid_until DATE,
    ADD COLUMN client_snapshot JSONB,
    ADD COLUMN project_description TEXT,
    ADD COLUMN scope_of_work TEXT,
    ADD COLUMN maintenance_info TEXT,
    ADD COLUMN hosting_info TEXT,
    ADD COLUMN notes TEXT,
    ADD COLUMN signature_image_url TEXT,
    ADD COLUMN product_detail_data JSONB;

-- ── 4. Drop new quotation indexes ────────────────────────────
DROP INDEX IF EXISTS idx_quotations_public_token;
DROP INDEX IF EXISTS idx_quotations_company_status;
DROP INDEX IF EXISTS idx_quotations_company_id;

-- ── 3. Drop CHECK constraint ─────────────────────────────────
ALTER TABLE quotations
    DROP CONSTRAINT IF EXISTS chk_quotations_commercial_scheme;

-- ── 2. Drop new quotation columns ────────────────────────────
ALTER TABLE quotations
    DROP COLUMN IF EXISTS active_published_revision_id,
    DROP COLUMN IF EXISTS public_token,
    DROP COLUMN IF EXISTS commercial_scheme;

-- ── 1. Rename solution_type → project_type ───────────────────
ALTER TABLE quotations RENAME COLUMN solution_type TO project_type;
