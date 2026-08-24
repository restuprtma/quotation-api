-- ============================================================
-- Phase 4 Session 1: AI-First Quotation Foundation
-- Destructive migration — drops legacy columns, renames, adds new schema
-- ============================================================

-- ── 1. QUOTATIONS: Rename project_type → solution_type ──────
ALTER TABLE quotations RENAME COLUMN project_type TO solution_type;

-- ── 2. QUOTATIONS: Add new columns ──────────────────────────
ALTER TABLE quotations
    ADD COLUMN commercial_scheme VARCHAR NOT NULL DEFAULT 'dedicated_squad',
    ADD COLUMN public_token UUID UNIQUE NOT NULL DEFAULT gen_random_uuid(),
    ADD COLUMN active_published_revision_id UUID NULL;

-- ── 3. QUOTATIONS: CHECK constraint ─────────────────────────
ALTER TABLE quotations
    ADD CONSTRAINT chk_quotations_commercial_scheme
    CHECK (commercial_scheme = 'dedicated_squad');

-- ── 4. QUOTATIONS: New indexes ──────────────────────────────
CREATE INDEX idx_quotations_company_id ON quotations(company_id, id);
CREATE INDEX idx_quotations_company_status ON quotations(company_id, status);
CREATE INDEX idx_quotations_public_token ON quotations(public_token);

-- ── 5. REVISIONS: Drop legacy columns ───────────────────────
ALTER TABLE quotation_revisions
    DROP COLUMN IF EXISTS public_token,
    DROP COLUMN IF EXISTS quotation_date,
    DROP COLUMN IF EXISTS valid_until,
    DROP COLUMN IF EXISTS client_snapshot,
    DROP COLUMN IF EXISTS project_description,
    DROP COLUMN IF EXISTS scope_of_work,
    DROP COLUMN IF EXISTS maintenance_info,
    DROP COLUMN IF EXISTS hosting_info,
    DROP COLUMN IF EXISTS notes,
    DROP COLUMN IF EXISTS signature_image_url,
    DROP COLUMN IF EXISTS product_detail_data;

-- ── 6. REVISIONS: Rename budget → budget_maximum ────────────
ALTER TABLE quotation_revisions RENAME COLUMN budget TO budget_maximum;

-- ── 7. REVISIONS: Add new columns ───────────────────────────
ALTER TABLE quotation_revisions
    ADD COLUMN idempotency_key UUID NULL,
    ADD COLUMN publication_status VARCHAR NOT NULL DEFAULT 'draft',
    ADD COLUMN generation_status VARCHAR NOT NULL DEFAULT 'pending',
    ADD COLUMN generation_input JSONB NOT NULL DEFAULT '{}',
    ADD COLUMN allocation_snapshot JSONB NULL,
    ADD COLUMN rate_card_snapshot JSONB NULL,
    ADD COLUMN legal_template_snapshot JSONB NULL,
    ADD COLUMN content_html TEXT NOT NULL DEFAULT '',
    ADD COLUMN content_version INTEGER NOT NULL DEFAULT 1,
    ADD COLUMN content_hash VARCHAR NULL,
    ADD COLUMN ai_provider VARCHAR NULL,
    ADD COLUMN ai_model VARCHAR NULL,
    ADD COLUMN skill_snapshot TEXT NULL,
    ADD COLUMN safety_envelope_version VARCHAR NULL,
    ADD COLUMN legal_content_modified BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN legal_modified_by UUID NULL,
    ADD COLUMN legal_modified_at TIMESTAMPTZ NULL,
    ADD COLUMN commercial_content_modified BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN commercial_modified_by UUID NULL,
    ADD COLUMN commercial_modified_at TIMESTAMPTZ NULL,
    ADD COLUMN generated_at TIMESTAMPTZ NULL,
    ADD COLUMN published_at TIMESTAMPTZ NULL,
    ADD COLUMN signer_name VARCHAR NULL,
    ADD COLUMN signer_email VARCHAR NULL,
    ADD COLUMN signature_data TEXT NULL,
    ADD COLUMN signed_content_hash VARCHAR NULL,
    ADD COLUMN signed_user_agent TEXT NULL;

-- ── 7b. DATA BACKFILL: Align publication_status for existing rows ──
UPDATE quotation_revisions
SET publication_status = 'superseded'
WHERE revision_number < (
    SELECT q.current_revision 
    FROM quotations q 
    WHERE q.id = quotation_revisions.quotation_id
);

UPDATE quotation_revisions
SET publication_status = 'published'
WHERE revision_number = (
    SELECT q.current_revision 
    FROM quotations q 
    WHERE q.id = quotation_revisions.quotation_id
) AND signed_at IS NOT NULL;

UPDATE quotations q
SET active_published_revision_id = r.id
FROM quotation_revisions r
WHERE r.quotation_id = q.id
  AND r.revision_number = q.current_revision
  AND r.signed_at IS NOT NULL;

-- ── 8. REVISIONS: Constraints ───────────────────────────────
ALTER TABLE quotation_revisions
    ADD CONSTRAINT uq_revision_number
    UNIQUE (quotation_id, revision_number);

CREATE UNIQUE INDEX uq_revisions_idempotency_key
    ON quotation_revisions (company_id, idempotency_key)
    WHERE idempotency_key IS NOT NULL;

CREATE UNIQUE INDEX uq_revisions_one_draft_per_quotation
    ON quotation_revisions (quotation_id)
    WHERE publication_status = 'draft';

CREATE UNIQUE INDEX uq_revisions_one_published_per_quotation
    ON quotation_revisions (quotation_id)
    WHERE publication_status = 'published';

ALTER TABLE quotation_revisions
    ADD CONSTRAINT chk_budget_maximum_non_negative
    CHECK (budget_maximum >= 0);

ALTER TABLE quotation_revisions
    ADD CONSTRAINT chk_content_version_positive
    CHECK (content_version >= 1);

-- ── 9. FK: active_published_revision_id (after revision table is ready) ──
ALTER TABLE quotations
    ADD CONSTRAINT fk_quotations_active_published_revision
    FOREIGN KEY (active_published_revision_id)
    REFERENCES quotation_revisions(id);
