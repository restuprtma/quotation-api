ALTER TABLE blueprint_revisions
    ADD COLUMN idempotency_payload_hash CHAR(64) NULL;

ALTER TABLE blueprint_revisions
    ADD CONSTRAINT chk_bp_idempotency_payload_hash CHECK (
        idempotency_payload_hash IS NULL OR idempotency_payload_hash ~ '^[0-9a-f]{64}$'
    ),
    ADD CONSTRAINT chk_bp_publication_status CHECK (
        publication_status IN ('draft', 'published', 'superseded')
    ),
    ADD CONSTRAINT chk_bp_generation_status CHECK (
        generation_status IN ('pending', 'in_progress', 'partial_failed', 'completed', 'failed')
    );

ALTER TABLE blueprint_section_runs
    ADD CONSTRAINT chk_bp_section_status CHECK (
        status IN ('pending', 'generating', 'completed', 'failed', 'manual_edit', 'validated')
    );

CREATE INDEX idx_blueprint_revisions_company_blueprint
    ON blueprint_revisions(company_id, blueprint_id);
CREATE INDEX idx_blueprint_section_runs_company_revision
    ON blueprint_section_runs(company_id, blueprint_revision_id);
