CREATE TABLE IF NOT EXISTS quotation_signature_settings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id UUID NOT NULL REFERENCES core.companies(id) ON DELETE CASCADE,
    director_name VARCHAR(255) NOT NULL DEFAULT 'Yulianto Frandi',
    director_title VARCHAR(255) NOT NULL DEFAULT 'Direksi Venturo Pro',
    director_signature_data TEXT NOT NULL DEFAULT '',
    company_stamp_data TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    CONSTRAINT uq_quotation_signature_settings_company UNIQUE(company_id)
);

CREATE INDEX IF NOT EXISTS idx_signature_settings_company ON quotation_signature_settings(company_id);

ALTER TABLE quotation_revisions
ADD COLUMN IF NOT EXISTS director_signature_snapshot TEXT,
ADD COLUMN IF NOT EXISTS director_stamp_snapshot TEXT;
