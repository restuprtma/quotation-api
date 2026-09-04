ALTER TABLE quotation_revisions
    DROP COLUMN IF EXISTS timeline_snapshot,
    DROP COLUMN IF EXISTS module_snapshot,
    DROP COLUMN IF EXISTS calculation_snapshot;
