-- Note: Data rows in these dropped tables and columns cannot be restored automatically.
-- This down migration only restores the structure to allow rollback code to start.

CREATE TABLE IF NOT EXISTS quotation_items (
    id UUID PRIMARY KEY,
    quotation_id UUID NOT NULL REFERENCES quotations(id) ON DELETE CASCADE,
    revision_number INT NOT NULL,
    item_name VARCHAR(255) NOT NULL,
    pricing_mode VARCHAR(50) NOT NULL,
    billing_period VARCHAR(50) NOT NULL,
    unit_price BIGINT NOT NULL,
    quantity INT NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS quotation_public_artifacts (
    id UUID PRIMARY KEY,
    company_id UUID NOT NULL,
    revision_id UUID NOT NULL REFERENCES quotation_revisions(id) ON DELETE CASCADE,
    artifact_type VARCHAR(50) NOT NULL,
    public_token UUID NOT NULL UNIQUE,
    published_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS quotation_pricing_sections (
    id UUID PRIMARY KEY,
    company_id UUID NOT NULL,
    quotation_id UUID NOT NULL REFERENCES quotations(id) ON DELETE CASCADE,
    revision_number INT NOT NULL,
    section_name VARCHAR(255) NOT NULL,
    section_type VARCHAR(50) NOT NULL,
    sort_order INT NOT NULL DEFAULT 0,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS quotation_pricing_items (
    id UUID PRIMARY KEY,
    company_id UUID NOT NULL,
    section_id UUID NOT NULL REFERENCES quotation_pricing_sections(id) ON DELETE CASCADE,
    item_name VARCHAR(255) NOT NULL,
    pricing_mode VARCHAR(50) NOT NULL,
    billing_period VARCHAR(50) NOT NULL,
    unit_price BIGINT NOT NULL,
    quantity INT NOT NULL,
    sort_order INT NOT NULL DEFAULT 0,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

ALTER TABLE quotation_revisions ADD COLUMN IF NOT EXISTS ai_raw_response TEXT;
ALTER TABLE quotation_revisions ADD COLUMN IF NOT EXISTS blueprint_data JSONB;
ALTER TABLE quotation_revisions ADD COLUMN IF NOT EXISTS narrative_data JSONB;
ALTER TABLE quotation_revisions ADD COLUMN IF NOT EXISTS product_detail JSONB;
ALTER TABLE quotation_revisions ADD COLUMN IF NOT EXISTS project_detail JSONB;
