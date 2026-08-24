-- ============================================================
-- Phase 4 Session 2: Support Tables & Share Links
-- Creates ai_skills, ai_credentials, rate_cards, rate_card_items,
-- legal_templates, blueprint_share_links
-- ============================================================

-- ── 1. AI SKILLS table ──────────────────────────────────────
CREATE TABLE IF NOT EXISTS ai_skills (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id UUID NOT NULL,
    name VARCHAR NOT NULL,
    command VARCHAR NOT NULL,
    content_instruction TEXT NOT NULL,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_by UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMPTZ NULL
);

CREATE UNIQUE INDEX uq_skills_active_command
    ON ai_skills (company_id, command)
    WHERE is_active = TRUE AND deleted_at IS NULL;

CREATE INDEX idx_skills_company ON ai_skills(company_id);

-- ── 2. AI CREDENTIALS table ─────────────────────────────────
CREATE TABLE IF NOT EXISTS ai_credentials (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id UUID NOT NULL,
    provider VARCHAR NOT NULL,
    model VARCHAR NOT NULL,
    encrypted_api_key TEXT NOT NULL,
    key_hint VARCHAR NOT NULL,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_by UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX uq_credentials_active_per_company
    ON ai_credentials (company_id)
    WHERE is_active = TRUE;

CREATE INDEX idx_credentials_company ON ai_credentials(company_id);

-- ── 3. RATE CARDS table ─────────────────────────────────────
CREATE TABLE IF NOT EXISTS rate_cards (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id UUID NOT NULL,
    code VARCHAR NOT NULL,
    name VARCHAR NOT NULL,
    market VARCHAR NOT NULL,
    currency VARCHAR NOT NULL DEFAULT 'IDR',
    effective_from DATE NOT NULL,
    effective_until DATE NULL,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_by UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT chk_rate_cards_market CHECK (market IN ('national', 'international')),
    CONSTRAINT uq_rate_cards_company_code UNIQUE (company_id, code)
);

CREATE INDEX idx_rate_cards_company ON rate_cards(company_id);

-- ── 4. RATE CARD ITEMS table ────────────────────────────────
CREATE TABLE IF NOT EXISTS rate_card_items (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id UUID NOT NULL,
    rate_card_id UUID NOT NULL REFERENCES rate_cards(id) ON DELETE CASCADE,
    role VARCHAR NOT NULL,
    seniority VARCHAR NOT NULL,
    term_type VARCHAR NOT NULL,
    amount_per_month BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT chk_rate_card_items_term_type CHECK (term_type IN ('short_term', 'long_term')),
    CONSTRAINT chk_rate_card_items_amount_positive CHECK (amount_per_month > 0),
    CONSTRAINT uq_rate_card_items_unique UNIQUE (company_id, rate_card_id, role, seniority, term_type)
);

CREATE INDEX idx_rate_card_items_rate_card ON rate_card_items(rate_card_id);

-- ── 5. LEGAL TEMPLATES table ────────────────────────────────
CREATE TABLE IF NOT EXISTS legal_templates (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id UUID NOT NULL,
    code VARCHAR NOT NULL,
    version INTEGER NOT NULL,
    content_html TEXT NOT NULL,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    approved_by UUID NULL,
    approved_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT uq_legal_templates_versioned UNIQUE (company_id, code, version)
);

CREATE INDEX idx_legal_templates_company ON legal_templates(company_id);

-- ── 6. BLUEPRINT SHARE LINKS table ──────────────────────────
CREATE TABLE IF NOT EXISTS blueprint_share_links (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id UUID NOT NULL,
    blueprint_id UUID NOT NULL REFERENCES blueprints(id),
    blueprint_revision_id UUID NOT NULL REFERENCES blueprint_revisions(id),
    token_hash VARCHAR NOT NULL UNIQUE,
    label VARCHAR NOT NULL,
    created_by UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    revoked_at TIMESTAMPTZ NULL,
    last_accessed_at TIMESTAMPTZ NULL
);

CREATE INDEX idx_share_links_company ON blueprint_share_links(company_id);
CREATE INDEX idx_share_links_blueprint ON blueprint_share_links(blueprint_id);
CREATE INDEX idx_share_links_token_hash ON blueprint_share_links(token_hash);
