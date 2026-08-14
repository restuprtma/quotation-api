-- file: 20260812_create_quotation_tables.up.sql

CREATE TABLE IF NOT EXISTS quotations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id UUID NOT NULL,
    client_id UUID NOT NULL,
    quotation_number VARCHAR(50) NOT NULL,
    project_type VARCHAR(20) NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'draft',
    current_revision INT DEFAULT 1,
    created_by UUID,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMPTZ
);

CREATE INDEX idx_quotations_company ON quotations(company_id);

CREATE TABLE IF NOT EXISTS quotation_revisions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id UUID NOT NULL,
    quotation_id UUID NOT NULL REFERENCES quotations(id) ON DELETE CASCADE,
    revision_number INT NOT NULL,
    public_token UUID UNIQUE NOT NULL DEFAULT gen_random_uuid(),
    budget BIGINT NOT NULL DEFAULT 0,
    project_description TEXT,
    scope_of_work TEXT,
    maintenance_info TEXT,
    hosting_info TEXT,
    notes TEXT,
    blueprint_data JSONB,
    ai_raw_response JSONB,
    signature_image_url VARCHAR(500),
    signed_at TIMESTAMPTZ,
    signed_ip VARCHAR(45),
    created_by UUID,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_revisions_company ON quotation_revisions(company_id);
CREATE INDEX idx_revisions_public_token ON quotation_revisions(public_token);

CREATE TABLE IF NOT EXISTS quotation_items (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id UUID NOT NULL,
    revision_id UUID NOT NULL REFERENCES quotation_revisions(id) ON DELETE CASCADE,
    item_name VARCHAR(200) NOT NULL,
    description TEXT,
    amount BIGINT NOT NULL DEFAULT 0,
    sort_order INT DEFAULT 0,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_items_revision ON quotation_items(revision_id);

CREATE TABLE IF NOT EXISTS contracts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id UUID NOT NULL,
    quotation_id UUID NOT NULL REFERENCES quotations(id) ON DELETE CASCADE,
    revision_id UUID NOT NULL REFERENCES quotation_revisions(id),
    contract_number VARCHAR(50) NOT NULL,
    public_token UUID UNIQUE NOT NULL DEFAULT gen_random_uuid(),
    status VARCHAR(20) NOT NULL DEFAULT 'draft',
    additional_terms TEXT,
    signature_image_url VARCHAR(500),
    signed_at TIMESTAMPTZ,
    signed_ip VARCHAR(45),
    created_by UUID,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMPTZ
);

CREATE INDEX idx_contracts_company ON contracts(company_id);
CREATE INDEX idx_contracts_public_token ON contracts(public_token);
