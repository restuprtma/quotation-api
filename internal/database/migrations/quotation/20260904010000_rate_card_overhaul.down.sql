-- Reverse infrastructure_packages
DROP TABLE IF EXISTS infrastructure_packages;

-- Reverse overtime_rate_configs
DROP TABLE IF EXISTS overtime_rate_configs;

-- Reverse rate_card_items expansion
ALTER TABLE rate_card_items
    DROP CONSTRAINT IF EXISTS chk_rate_card_items_discipline,
    DROP COLUMN IF EXISTS discipline,
    DROP COLUMN IF EXISTS display_name,
    DROP COLUMN IF EXISTS education_requirement,
    DROP COLUMN IF EXISTS experience_requirement,
    DROP COLUMN IF EXISTS base_salary,
    DROP COLUMN IF EXISTS tech_lead_fee,
    DROP COLUMN IF EXISTS ai_rnd_fee,
    DROP COLUMN IF EXISTS workspace_fee,
    DROP COLUMN IF EXISTS management_fee,
    DROP COLUMN IF EXISTS published_rate,
    DROP COLUMN IF EXISTS max_discount_pct,
    DROP COLUMN IF EXISTS floor_rate,
    DROP COLUMN IF EXISTS kickoff_fee,
    DROP COLUMN IF EXISTS recruitment_fee,
    DROP COLUMN IF EXISTS onboarding_fee;

-- Reverse rate_cards expansion
ALTER TABLE rate_cards
    DROP CONSTRAINT IF EXISTS chk_rate_cards_client_tier,
    DROP COLUMN IF EXISTS client_tier,
    DROP COLUMN IF EXISTS description,
    DROP COLUMN IF EXISTS short_term_surcharge_pct,
    DROP COLUMN IF EXISTS enterprise_markup_pct;
