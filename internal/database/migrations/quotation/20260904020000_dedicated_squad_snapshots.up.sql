ALTER TABLE quotation_revisions
    ADD COLUMN calculation_snapshot JSONB NULL,
    ADD COLUMN module_snapshot JSONB NULL,
    ADD COLUMN timeline_snapshot JSONB NULL;
