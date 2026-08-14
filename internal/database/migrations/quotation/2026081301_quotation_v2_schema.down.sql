-- 1. Drop quotation_public_artifacts
DROP TABLE IF EXISTS quotation_public_artifacts;

-- 2. Drop quotation_pricing_items
DROP TABLE IF EXISTS quotation_pricing_items;

-- 3. Drop quotation_pricing_sections
DROP TABLE IF EXISTS quotation_pricing_sections;

-- 4. Drop columns from quotation_revisions
ALTER TABLE quotation_revisions
    DROP COLUMN IF EXISTS quotation_date,
    DROP COLUMN IF EXISTS valid_until,
    DROP COLUMN IF EXISTS client_snapshot,
    DROP COLUMN IF EXISTS narrative_data,
    DROP COLUMN IF EXISTS product_detail_data;

-- 5. Drop column from quotations
ALTER TABLE quotations
    DROP COLUMN IF EXISTS subject;
