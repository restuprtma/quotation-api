-- ============================================================
-- Phase 4 Session 2: Blueprint Foundation
-- Creates blueprints, blueprint_revisions, blueprint_section_runs
-- ============================================================

-- ── 1. BLUEPRINTS table ─────────────────────────────────────
CREATE TABLE IF NOT EXISTS blueprints (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id UUID NOT NULL,
    quotation_id UUID NOT NULL UNIQUE REFERENCES quotations(id),
    title VARCHAR NOT NULL,
    status VARCHAR NOT NULL DEFAULT 'draft',
    current_revision INTEGER NOT NULL DEFAULT 1,
    active_published_revision_id UUID NULL,
    created_by UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMPTZ NULL
);

CREATE INDEX idx_blueprints_company ON blueprints(company_id);
CREATE INDEX idx_blueprints_quotation ON blueprints(quotation_id);

-- ── 2. BLUEPRINT REVISIONS table ────────────────────────────
CREATE TABLE IF NOT EXISTS blueprint_revisions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id UUID NOT NULL,
    blueprint_id UUID NOT NULL REFERENCES blueprints(id) ON DELETE CASCADE,
    revision_number INTEGER NOT NULL,
    idempotency_key UUID NULL,
    source_quotation_revision_id UUID NOT NULL REFERENCES quotation_revisions(id),
    source_context_snapshot JSONB NOT NULL,
    publication_status VARCHAR NOT NULL DEFAULT 'draft',
    completeness_status VARCHAR NOT NULL DEFAULT 'partial',
    generation_status VARCHAR NOT NULL DEFAULT 'pending',
    content_json JSONB NOT NULL DEFAULT '{}',
    content_version INTEGER NOT NULL DEFAULT 1,
    validation_result JSONB NOT NULL DEFAULT '{}',
    cross_validation_result JSONB NOT NULL DEFAULT '{}',
    skill_snapshot TEXT NULL,
    safety_envelope_version VARCHAR NULL,
    ai_provider VARCHAR NULL,
    ai_model VARCHAR NULL,
    generated_at TIMESTAMPTZ NULL,
    published_at TIMESTAMPTZ NULL,
    created_by UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- ── 3. BLUEPRINT SECTION RUNS table ─────────────────────────
CREATE TABLE IF NOT EXISTS blueprint_section_runs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id UUID NOT NULL,
    blueprint_revision_id UUID NOT NULL REFERENCES blueprint_revisions(id) ON DELETE CASCADE,
    section_type VARCHAR NOT NULL,
    status VARCHAR NOT NULL DEFAULT 'pending',
    attempt_count INTEGER NOT NULL DEFAULT 0,
    content_json JSONB NULL,
    validation_result JSONB NOT NULL DEFAULT '{}',
    safe_error_code VARCHAR NULL,
    started_at TIMESTAMPTZ NULL,
    completed_at TIMESTAMPTZ NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT chk_section_type CHECK (section_type IN (
        'context', 'journey', 'architecture', 'state_machine',
        'workflow', 'wbs', 'cross_validation'
    )),
    CONSTRAINT uq_section_run_per_revision UNIQUE (blueprint_revision_id, section_type)
);

CREATE INDEX idx_section_runs_revision ON blueprint_section_runs(blueprint_revision_id);

-- ── 4. BLUEPRINT REVISION Constraints & Indexes ─────────────
ALTER TABLE blueprint_revisions
    ADD CONSTRAINT uq_blueprint_revision_number
    UNIQUE (blueprint_id, revision_number);

CREATE UNIQUE INDEX uq_blueprint_revisions_one_draft
    ON blueprint_revisions (blueprint_id)
    WHERE publication_status = 'draft';

CREATE UNIQUE INDEX uq_blueprint_revisions_one_published
    ON blueprint_revisions (blueprint_id)
    WHERE publication_status = 'published';

CREATE UNIQUE INDEX uq_blueprint_revisions_idempotency_key
    ON blueprint_revisions (company_id, idempotency_key)
    WHERE idempotency_key IS NOT NULL;

ALTER TABLE blueprint_revisions
    ADD CONSTRAINT chk_bp_completeness_status
    CHECK (completeness_status IN ('partial', 'complete'));

ALTER TABLE blueprint_revisions
    ADD CONSTRAINT chk_bp_content_version_positive
    CHECK (content_version >= 1);

-- ── 5. FK: blueprints.active_published_revision_id ──────────
ALTER TABLE blueprints
    ADD CONSTRAINT fk_blueprints_active_published_revision
    FOREIGN KEY (active_published_revision_id)
    REFERENCES blueprint_revisions(id);
