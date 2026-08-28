CREATE TABLE ai_credentials (
    company_id UUID NOT NULL,
    provider VARCHAR(50) NOT NULL,
    encrypted_api_key TEXT NOT NULL,
    key_hint VARCHAR(50) NOT NULL,
    is_active BOOLEAN DEFAULT false,
    timeout_ms INT DEFAULT 30000,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT uq_company_provider UNIQUE (company_id, provider)
);

CREATE INDEX idx_ai_credentials_company_active ON ai_credentials (company_id, is_active);

