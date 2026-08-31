ALTER TABLE quotation_revisions ADD COLUMN IF NOT EXISTS generation_error TEXT NULL;
-- Add a snapshot column just in case the master client name changes later
ALTER TABLE quotations ADD COLUMN IF NOT EXISTS client_name_snapshot VARCHAR(255) NULL;
