-- ============================================================
-- Phase 6 Session 5: Regeneration cost-control columns
-- regenerate_count/last_regenerate_at power the race-safe
-- max-5-per-hour-per-draft guard enforced by a single gated
-- atomic UPDATE in GenerationRepository.ConsumeRegenerateQuota.
-- ============================================================

ALTER TABLE quotation_revisions
    ADD COLUMN regenerate_count INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN last_regenerate_at TIMESTAMPTZ NULL;

ALTER TABLE quotation_revisions
    ADD CONSTRAINT chk_regenerate_count_non_negative
    CHECK (regenerate_count >= 0);
