-- ── 1. Expand rate_cards: add client_tier support ────────────
ALTER TABLE rate_cards
    DROP CONSTRAINT IF EXISTS chk_rate_cards_market;

ALTER TABLE rate_cards
    ADD COLUMN IF NOT EXISTS client_tier VARCHAR NOT NULL DEFAULT 'commercial_national',
    ADD COLUMN IF NOT EXISTS description TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS short_term_surcharge_pct INTEGER NOT NULL DEFAULT 40,
    ADD COLUMN IF NOT EXISTS enterprise_markup_pct INTEGER NOT NULL DEFAULT 35;

ALTER TABLE rate_cards
    ADD CONSTRAINT chk_rate_cards_market CHECK (market IN ('national', 'international'));

ALTER TABLE rate_cards
    ADD CONSTRAINT chk_rate_cards_client_tier CHECK (client_tier IN ('commercial_national', 'government_bumn', 'international'));

-- ── 2. Expand rate_card_items: cost breakdown + negotiation ──
ALTER TABLE rate_card_items
    ADD COLUMN IF NOT EXISTS discipline VARCHAR NOT NULL DEFAULT 'ENGINEERING',
    ADD COLUMN IF NOT EXISTS display_name VARCHAR NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS education_requirement VARCHAR NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS experience_requirement VARCHAR NOT NULL DEFAULT '',
    -- Cost breakdown components (integer IDR)
    ADD COLUMN IF NOT EXISTS base_salary BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS tech_lead_fee BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS ai_rnd_fee BIGINT NOT NULL DEFAULT 500000,
    ADD COLUMN IF NOT EXISTS workspace_fee BIGINT NOT NULL DEFAULT 500000,
    ADD COLUMN IF NOT EXISTS management_fee BIGINT NOT NULL DEFAULT 0,
    -- Negotiation parameters
    ADD COLUMN IF NOT EXISTS published_rate BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS max_discount_pct INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS floor_rate BIGINT NOT NULL DEFAULT 0,
    -- Preparation fees
    ADD COLUMN IF NOT EXISTS kickoff_fee BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS recruitment_fee BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS onboarding_fee BIGINT NOT NULL DEFAULT 0;

ALTER TABLE rate_card_items
    ADD CONSTRAINT chk_rate_card_items_discipline CHECK (
        discipline IN ('ENGINEERING', 'QA', 'PRODUCT', 'DATA', 'INFRASTRUCTURE', 'SUPPORT')
    );

-- ── 3. Overtime rate configuration ──────────────────────────
CREATE TABLE IF NOT EXISTS overtime_rate_configs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id UUID NOT NULL,
    rate_card_id UUID NOT NULL REFERENCES rate_cards(id) ON DELETE CASCADE,
    role VARCHAR NOT NULL,
    seniority VARCHAR NOT NULL,
    term_type VARCHAR NOT NULL,
    base_monthly_rate BIGINT NOT NULL,
    hourly_divisor INTEGER NOT NULL DEFAULT 173,
    workday_first_hour_multiplier INTEGER NOT NULL DEFAULT 150,
    workday_subsequent_multiplier INTEGER NOT NULL DEFAULT 200,
    holiday_base_multiplier INTEGER NOT NULL DEFAULT 200,
    holiday_9th_hour_multiplier INTEGER NOT NULL DEFAULT 300,
    holiday_10plus_multiplier INTEGER NOT NULL DEFAULT 400,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT chk_overtime_term_type CHECK (term_type IN ('short_term', 'long_term')),
    CONSTRAINT uq_overtime_config UNIQUE (company_id, rate_card_id, role, seniority, term_type)
);

CREATE INDEX idx_overtime_rate_card ON overtime_rate_configs(rate_card_id);

-- ── 4. Infrastructure packages ──────────────────────────────
CREATE TABLE IF NOT EXISTS infrastructure_packages (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id UUID NOT NULL,
    rate_card_id UUID NOT NULL REFERENCES rate_cards(id) ON DELETE CASCADE,
    category VARCHAR NOT NULL,
    package_name VARCHAR NOT NULL,
    vcpu INTEGER NOT NULL DEFAULT 0,
    ram_gb INTEGER NOT NULL DEFAULT 0,
    storage_gb INTEGER NOT NULL DEFAULT 0,
    amount_per_month BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT chk_infra_category CHECK (category IN ('shared_hosting', 'vps_cloud')),
    CONSTRAINT chk_infra_amount_positive CHECK (amount_per_month > 0),
    CONSTRAINT uq_infra_package UNIQUE (company_id, rate_card_id, category, package_name)
);

CREATE INDEX idx_infra_rate_card ON infrastructure_packages(rate_card_id);
