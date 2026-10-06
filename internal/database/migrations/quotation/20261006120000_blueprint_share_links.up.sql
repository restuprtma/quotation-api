-- Phase 12: Secure Blueprint Share Links table and partial active token index
CREATE TABLE IF NOT EXISTS blueprint_share_links (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id UUID NOT NULL,
    blueprint_id UUID NOT NULL REFERENCES blueprints(id) ON DELETE CASCADE,
    blueprint_revision_id UUID NOT NULL REFERENCES blueprint_revisions(id) ON DELETE CASCADE,
    token_hash VARCHAR(64) NOT NULL,
    label VARCHAR(255) NOT NULL,
    created_by UUID NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    revoked_at TIMESTAMPTZ NULL,
    last_accessed_at TIMESTAMPTZ NULL
);

-- Drop old unique constraint on token_hash if it exists from earlier prototype schemas
ALTER TABLE blueprint_share_links DROP CONSTRAINT IF EXISTS blueprint_share_links_token_hash_key;
DROP INDEX IF EXISTS idx_share_links_token_hash;
DROP INDEX IF EXISTS idx_share_links_blueprint;
DROP INDEX IF EXISTS idx_share_links_company;

-- Unique partial index for non-revoked active token hashes
CREATE UNIQUE INDEX IF NOT EXISTS uq_blueprint_share_links_token_hash_active
    ON blueprint_share_links (token_hash)
    WHERE revoked_at IS NULL;

-- Composite index for tenant-scoped list and revoke queries
CREATE INDEX IF NOT EXISTS idx_blueprint_share_links_blueprint_company
    ON blueprint_share_links (blueprint_id, company_id);

-- Lookup index for token hash lookups
CREATE INDEX IF NOT EXISTS idx_blueprint_share_links_token_hash
    ON blueprint_share_links (token_hash);
