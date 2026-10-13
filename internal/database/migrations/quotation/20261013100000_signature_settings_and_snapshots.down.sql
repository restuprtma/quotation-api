ALTER TABLE quotation_revisions
DROP COLUMN IF EXISTS director_signature_snapshot,
DROP COLUMN IF EXISTS director_stamp_snapshot;

DROP TABLE IF EXISTS quotation_signature_settings;
