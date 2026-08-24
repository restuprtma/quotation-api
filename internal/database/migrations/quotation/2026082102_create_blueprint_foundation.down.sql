-- ============================================================
-- Phase 4 Session 2: Blueprint Foundation (DOWN)
-- Reverts blueprints, blueprint_revisions, blueprint_section_runs
-- ============================================================

-- ── 1. Drop FK from blueprints ──────────────────────────────
ALTER TABLE blueprints
    DROP CONSTRAINT IF EXISTS fk_blueprints_active_published_revision;

-- ── 2. Drop blueprint_revisions constraints & indexes ───────
ALTER TABLE blueprint_revisions
    DROP CONSTRAINT IF EXISTS chk_bp_content_version_positive;

ALTER TABLE blueprint_revisions
    DROP CONSTRAINT IF EXISTS chk_bp_completeness_status;

DROP INDEX IF EXISTS uq_blueprint_revisions_idempotency_key;
DROP INDEX IF EXISTS uq_blueprint_revisions_one_published;
DROP INDEX IF EXISTS uq_blueprint_revisions_one_draft;

ALTER TABLE blueprint_revisions
    DROP CONSTRAINT IF EXISTS uq_blueprint_revision_number;

-- ── 3. Drop tables in dependency order ──────────────────────
DROP TABLE IF EXISTS blueprint_section_runs;
DROP TABLE IF EXISTS blueprint_revisions;
DROP TABLE IF EXISTS blueprints;
