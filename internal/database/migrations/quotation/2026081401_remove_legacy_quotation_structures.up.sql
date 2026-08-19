DROP TABLE IF EXISTS quotation_pricing_items;
DROP TABLE IF EXISTS quotation_pricing_sections;
DROP TABLE IF EXISTS quotation_public_artifacts;
DROP TABLE IF EXISTS quotation_items;

ALTER TABLE quotation_revisions DROP COLUMN IF EXISTS ai_raw_response;
ALTER TABLE quotation_revisions DROP COLUMN IF EXISTS blueprint_data;
ALTER TABLE quotation_revisions DROP COLUMN IF EXISTS narrative_data;
ALTER TABLE quotation_revisions DROP COLUMN IF EXISTS product_detail;
ALTER TABLE quotation_revisions DROP COLUMN IF EXISTS project_detail;
