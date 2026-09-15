DROP INDEX IF EXISTS idx_blueprint_section_runs_company_revision;
DROP INDEX IF EXISTS idx_blueprint_revisions_company_blueprint;
ALTER TABLE blueprint_section_runs DROP CONSTRAINT IF EXISTS chk_bp_section_status;
ALTER TABLE blueprint_revisions
    DROP CONSTRAINT IF EXISTS chk_bp_generation_status,
    DROP CONSTRAINT IF EXISTS chk_bp_publication_status,
    DROP CONSTRAINT IF EXISTS chk_bp_idempotency_payload_hash,
    DROP COLUMN IF EXISTS idempotency_payload_hash;
