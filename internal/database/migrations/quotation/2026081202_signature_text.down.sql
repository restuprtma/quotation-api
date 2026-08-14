-- Rollback can fail when stored data URLs exceed VARCHAR(500).
ALTER TABLE quotation_revisions
    ALTER COLUMN signature_image_url TYPE VARCHAR(500);
