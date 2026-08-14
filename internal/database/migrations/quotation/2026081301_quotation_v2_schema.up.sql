-- 1. Alter quotations: add subject
ALTER TABLE quotations ADD COLUMN subject TEXT;

-- 2. Alter quotation_revisions: add v2 fields
ALTER TABLE quotation_revisions 
    ADD COLUMN quotation_date DATE,
    ADD COLUMN valid_until DATE,
    ADD COLUMN client_snapshot JSONB,
    ADD COLUMN narrative_data JSONB,
    ADD COLUMN product_detail_data JSONB;

-- 3. Create quotation_pricing_sections
CREATE TABLE IF NOT EXISTS quotation_pricing_sections (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id UUID NOT NULL,
    revision_id UUID NOT NULL REFERENCES quotation_revisions(id) ON DELETE CASCADE,
    section_name VARCHAR(200) NOT NULL,
    section_type VARCHAR(50) NOT NULL CHECK (section_type IN ('package', 'service', 'phase', 'resource_group', 'other')),
    description TEXT,
    sort_order INT DEFAULT 0,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_pricing_sections_company ON quotation_pricing_sections(company_id);
CREATE INDEX idx_pricing_sections_revision ON quotation_pricing_sections(revision_id);

-- 4. Create quotation_pricing_items
CREATE TABLE IF NOT EXISTS quotation_pricing_items (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id UUID NOT NULL,
    section_id UUID NOT NULL REFERENCES quotation_pricing_sections(id) ON DELETE CASCADE,
    item_name VARCHAR(200) NOT NULL,
    description TEXT,
    features JSONB,
    pricing_mode VARCHAR(20) NOT NULL CHECK (pricing_mode IN ('unit', 'resource', 'fixed')),
    unit_label VARCHAR(50),
    unit_price BIGINT NOT NULL DEFAULT 0,
    quantity BIGINT NOT NULL DEFAULT 0,
    employee_count INT NOT NULL DEFAULT 0,
    duration INT NOT NULL DEFAULT 0,
    duration_unit VARCHAR(50),
    billing_period VARCHAR(20) NOT NULL CHECK (billing_period IN ('one_time', 'monthly', 'yearly', 'custom')),
    line_total BIGINT NOT NULL DEFAULT 0,
    manual_override_reason TEXT,
    sort_order INT DEFAULT 0,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_pricing_items_company ON quotation_pricing_items(company_id);
CREATE INDEX idx_pricing_items_section ON quotation_pricing_items(section_id);

-- 5. Create quotation_public_artifacts
CREATE TABLE IF NOT EXISTS quotation_public_artifacts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id UUID NOT NULL,
    revision_id UUID NOT NULL REFERENCES quotation_revisions(id) ON DELETE CASCADE,
    artifact_type VARCHAR(50) NOT NULL CHECK (artifact_type IN ('quotation', 'blueprint', 'detail')),
    public_token UUID UNIQUE NOT NULL DEFAULT gen_random_uuid(),
    published_at TIMESTAMPTZ,
    revoked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (revision_id, artifact_type)
);

CREATE INDEX idx_public_artifacts_company ON quotation_public_artifacts(company_id);
CREATE INDEX idx_public_artifacts_revision ON quotation_public_artifacts(revision_id);
CREATE INDEX idx_public_artifacts_token ON quotation_public_artifacts(public_token);
