-- ============================================================
-- Phase 6 Session 5: Rollback regeneration cost-control columns
-- ============================================================

ALTER TABLE quotation_revisions
    DROP CONSTRAINT IF EXISTS chk_regenerate_count_non_negative,
    DROP COLUMN IF EXISTS last_regenerate_at,
    DROP COLUMN IF EXISTS regenerate_count;
