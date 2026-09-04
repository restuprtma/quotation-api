-- ============================================================================
-- VENTURO MASTER RATE CARDS SEEDER (001_venturo_master_rate_cards.sql)
-- Multi-tenant Master Rate Cards for Quotation Generator
-- Populates:
--   1. DEFAULT (Commercial National - Updated/Cleaned)
--   2. VENTURO-STD-2026 (Commercial National Standard Master)
--   3. VENTURO-BUMN-2026 (Enterprise & Government/BUMN Tier)
--   4. VENTURO-INTL-2026 (International Market Tier)
-- Includes:
--   - 14 Standard Roles x 3 Seniorities x 2 Term Types = 84 items per card
--   - 5-part Cost Breakdown (Base Salary, Tech Lead, AI R&D, Workspace, Mgmt)
--   - Negotiation Bounds & Preparation Fees (Kickoff, Recruitment, Onboarding)
--   - Overtime Configurations (Depnaker formula, divisor 173)
--   - Infrastructure Packages (3 Shared Hosting + 4 VPS Cloud Server)
-- ============================================================================

DO $$
DECLARE
    v_company_id UUID := '20000000-0000-0000-0000-000000000001'; -- PT Alpha Indonesia
    v_now TIMESTAMPTZ := CURRENT_TIMESTAMP;
    v_card_rec RECORD;
    v_role_rec RECORD;
    v_sen_rec RECORD;
    v_term VARCHAR;
    v_rc_id UUID;
    
    -- Item breakdown variables
    v_base_salary BIGINT;
    v_tech_lead BIGINT;
    v_ai_rnd BIGINT;
    v_workspace BIGINT;
    v_mgmt BIGINT;
    v_long_term_rate BIGINT;
    v_short_term_rate BIGINT;
    v_rate BIGINT;
    v_pub_rate BIGINT;
    v_max_disc INT;
    v_floor_rate BIGINT;
    v_kickoff BIGINT;
    v_recruitment BIGINT;
    v_onboarding BIGINT;
    
    -- Multiplier for tier
    v_tier_mult NUMERIC;
BEGIN
    RAISE NOTICE 'Starting Venturo Master Rate Cards Seeding for Company: %', v_company_id;

    -- ------------------------------------------------------------------------
    -- 1. Create or Update Rate Cards
    -- ------------------------------------------------------------------------
    -- Rate Card 1: DEFAULT (Commercial National)
    INSERT INTO rate_cards (
        id, company_id, code, name, client_tier, market, currency, description,
        short_term_surcharge_pct, enterprise_markup_pct, effective_from, is_active, created_at, updated_at
    ) VALUES (
        'dbab0d44-35f0-4ce6-9e3c-905a28c4b5a8',
        v_company_id,
        'DEFAULT',
        'Default Rate Card',
        'commercial_national',
        'national',
        'IDR',
        'Standard default commercial rate card for private and national commercial clients.',
        40, 35, '2026-01-01', TRUE, v_now, v_now
    )
    ON CONFLICT (company_id, code) DO UPDATE SET
        name = EXCLUDED.name,
        client_tier = EXCLUDED.client_tier,
        market = EXCLUDED.market,
        currency = EXCLUDED.currency,
        description = EXCLUDED.description,
        short_term_surcharge_pct = EXCLUDED.short_term_surcharge_pct,
        enterprise_markup_pct = EXCLUDED.enterprise_markup_pct,
        is_active = TRUE,
        updated_at = v_now;

    -- Rate Card 2: VENTURO-STD-2026 (Commercial National Master)
    INSERT INTO rate_cards (
        id, company_id, code, name, client_tier, market, currency, description,
        short_term_surcharge_pct, enterprise_markup_pct, effective_from, is_active, created_at, updated_at
    ) VALUES (
        'dbab0d44-35f0-4ce6-9e3c-905a28c4b5a9',
        v_company_id,
        'VENTURO-STD-2026',
        'Venturo National Commercial Master Rate Card 2026',
        'commercial_national',
        'national',
        'IDR',
        'Master reference rate card for commercial private clients with 5-part cost breakdown, overtime, and infra catalog.',
        40, 35, '2026-01-01', TRUE, v_now, v_now
    )
    ON CONFLICT (company_id, code) DO UPDATE SET
        name = EXCLUDED.name,
        client_tier = EXCLUDED.client_tier,
        market = EXCLUDED.market,
        currency = EXCLUDED.currency,
        description = EXCLUDED.description,
        short_term_surcharge_pct = EXCLUDED.short_term_surcharge_pct,
        enterprise_markup_pct = EXCLUDED.enterprise_markup_pct,
        is_active = TRUE,
        updated_at = v_now;

    -- Rate Card 3: VENTURO-BUMN-2026 (Government & BUMN Enterprise Tier)
    INSERT INTO rate_cards (
        id, company_id, code, name, client_tier, market, currency, description,
        short_term_surcharge_pct, enterprise_markup_pct, effective_from, is_active, created_at, updated_at
    ) VALUES (
        'dbab0d44-35f0-4ce6-9e3c-905a28c4b5b0',
        v_company_id,
        'VENTURO-BUMN-2026',
        'Venturo Enterprise & BUMN / Government Rate Card 2026',
        'government_bumn',
        'national',
        'IDR',
        'Official billing consultant rates for government, BUMN, and public institutions (+35% compliance & enterprise markup).',
        40, 35, '2026-01-01', TRUE, v_now, v_now
    )
    ON CONFLICT (company_id, code) DO UPDATE SET
        name = EXCLUDED.name,
        client_tier = EXCLUDED.client_tier,
        market = EXCLUDED.market,
        currency = EXCLUDED.currency,
        description = EXCLUDED.description,
        short_term_surcharge_pct = EXCLUDED.short_term_surcharge_pct,
        enterprise_markup_pct = EXCLUDED.enterprise_markup_pct,
        is_active = TRUE,
        updated_at = v_now;

    -- Rate Card 4: VENTURO-INTL-2026 (International Market Tier)
    INSERT INTO rate_cards (
        id, company_id, code, name, client_tier, market, currency, description,
        short_term_surcharge_pct, enterprise_markup_pct, effective_from, is_active, created_at, updated_at
    ) VALUES (
        'dbab0d44-35f0-4ce6-9e3c-905a28c4b5b1',
        v_company_id,
        'VENTURO-INTL-2026',
        'Venturo International Market Rate Card 2026',
        'international',
        'international',
        'IDR',
        'Global delivery and offshore staffing rate card for international clients (+50% offshore index).',
        40, 50, '2026-01-01', TRUE, v_now, v_now
    )
    ON CONFLICT (company_id, code) DO UPDATE SET
        name = EXCLUDED.name,
        client_tier = EXCLUDED.client_tier,
        market = EXCLUDED.market,
        currency = EXCLUDED.currency,
        description = EXCLUDED.description,
        short_term_surcharge_pct = EXCLUDED.short_term_surcharge_pct,
        enterprise_markup_pct = EXCLUDED.enterprise_markup_pct,
        is_active = TRUE,
        updated_at = v_now;

    -- ------------------------------------------------------------------------
    -- Clean existing items, overtime configs, and infra packages for target cards
    -- ------------------------------------------------------------------------
    DELETE FROM rate_card_items WHERE rate_card_id IN (
        'dbab0d44-35f0-4ce6-9e3c-905a28c4b5a8',
        'dbab0d44-35f0-4ce6-9e3c-905a28c4b5a9',
        'dbab0d44-35f0-4ce6-9e3c-905a28c4b5b0',
        'dbab0d44-35f0-4ce6-9e3c-905a28c4b5b1'
    );
    DELETE FROM overtime_rate_configs WHERE rate_card_id IN (
        'dbab0d44-35f0-4ce6-9e3c-905a28c4b5a8',
        'dbab0d44-35f0-4ce6-9e3c-905a28c4b5a9',
        'dbab0d44-35f0-4ce6-9e3c-905a28c4b5b0',
        'dbab0d44-35f0-4ce6-9e3c-905a28c4b5b1'
    );
    DELETE FROM infrastructure_packages WHERE rate_card_id IN (
        'dbab0d44-35f0-4ce6-9e3c-905a28c4b5a8',
        'dbab0d44-35f0-4ce6-9e3c-905a28c4b5a9',
        'dbab0d44-35f0-4ce6-9e3c-905a28c4b5b0',
        'dbab0d44-35f0-4ce6-9e3c-905a28c4b5b1'
    );

    -- ------------------------------------------------------------------------
    -- 2. Populate 14 Roles for Each Rate Card
    -- ------------------------------------------------------------------------
    FOR v_card_rec IN
        SELECT id, code, client_tier FROM rate_cards WHERE id IN (
            'dbab0d44-35f0-4ce6-9e3c-905a28c4b5a8',
            'dbab0d44-35f0-4ce6-9e3c-905a28c4b5a9',
            'dbab0d44-35f0-4ce6-9e3c-905a28c4b5b0',
            'dbab0d44-35f0-4ce6-9e3c-905a28c4b5b1'
        )
    LOOP
        v_rc_id := v_card_rec.id;
        
        -- Set tier multiplier
        IF v_card_rec.client_tier = 'government_bumn' THEN
            v_tier_mult := 1.35;
        ELSIF v_card_rec.client_tier = 'international' THEN
            v_tier_mult := 1.50;
        ELSE
            v_tier_mult := 1.00;
        END IF;

        -- Loop through 14 roles
        FOR v_role_rec IN
            SELECT * FROM (VALUES
                ('business_analyst', 'Business Analyst', 'PRODUCT', 'S1', '> 2 th', 'analysis', 5),
                ('system_analyst', 'System Analyst', 'ENGINEERING', 'S1', '> 2 th', 'analysis', 5),
                ('frontend_programmer', 'Front End Programmer', 'ENGINEERING', 'S1/D4', '> 2 th', 'engineering', 7),
                ('backend_programmer', 'Back End Programmer', 'ENGINEERING', 'S1/D4', '> 2 th', 'engineering', 5),
                ('fullstack_developer', 'Full Stack Developer', 'ENGINEERING', 'S1/D4', '> 2 th', 'engineering', 5),
                ('mobile_developer', 'Mobile Developer', 'ENGINEERING', 'S1/D4', '> 2 th', 'engineering', 5),
                ('devops', 'DevOps Engineer', 'INFRASTRUCTURE', 'S1/D4', '> 2 th', 'engineering', 8),
                ('data_engineer', 'Data Engineer', 'DATA', 'S1/D4', '> 2 th', 'analysis', 5),
                ('data_analyst', 'Data Analyst', 'DATA', 'S1/D4', '> 2 th', 'analysis', 0),
                ('ui_ux_designer', 'UI/UX Designer', 'PRODUCT', 'D3/S1', '> 2 th', 'engineering', 5),
                ('automated_tester', 'Automated Tester (QA Automation)', 'QA', 'D3/S1', '< 2 th', 'engineering', 5),
                ('functional_tester', 'Functional Tester (Manual QA)', 'QA', 'D3', '< 2 th', 'support', 10),
                ('technical_writer', 'Technical Writer', 'SUPPORT', 'D3', '> 2 th', 'support', 10),
                ('pentester', 'Penetration Tester', 'INFRASTRUCTURE', 'S1/Sertifikasi', '> 2 th', 'engineering', 0)
            ) AS t(role, display_name, discipline, education, experience, archetype, default_discount)
        LOOP
            -- Loop through Seniorities
            FOR v_sen_rec IN
                SELECT * FROM (VALUES
                    ('senior'),
                    ('intermediate'),
                    ('associate')
                ) AS s(seniority)
            LOOP
                -- Determine Base Components per archetype & seniority
                IF v_role_rec.archetype = 'analysis' THEN
                    IF v_sen_rec.seniority = 'senior' THEN
                        v_base_salary := ROUND(10000000 * v_tier_mult);
                        v_tech_lead := ROUND(1500000 * v_tier_mult);
                        v_ai_rnd := 500000;
                        v_workspace := 500000;
                        v_mgmt := ROUND(4200000 * v_tier_mult);
                        v_long_term_rate := v_base_salary + v_tech_lead + v_ai_rnd + v_workspace + v_mgmt;
                        v_short_term_rate := ROUND(v_long_term_rate * 1.40);
                        v_kickoff := v_long_term_rate;
                        v_recruitment := 3000000;
                        v_onboarding := 13600000;
                    ELSIF v_sen_rec.seniority = 'intermediate' THEN
                        v_base_salary := ROUND(8000000 * v_tier_mult);
                        v_tech_lead := ROUND(1200000 * v_tier_mult);
                        v_ai_rnd := 500000;
                        v_workspace := 500000;
                        v_mgmt := ROUND(3300000 * v_tier_mult);
                        v_long_term_rate := v_base_salary + v_tech_lead + v_ai_rnd + v_workspace + v_mgmt;
                        v_short_term_rate := ROUND(v_long_term_rate * 1.40);
                        v_kickoff := v_long_term_rate;
                        v_recruitment := 1500000;
                        v_onboarding := 11000000;
                    ELSE -- associate
                        v_base_salary := ROUND(5500000 * v_tier_mult);
                        v_tech_lead := ROUND(1000000 * v_tier_mult);
                        v_ai_rnd := 500000;
                        v_workspace := 500000;
                        v_mgmt := ROUND(2500000 * v_tier_mult);
                        v_long_term_rate := v_base_salary + v_tech_lead + v_ai_rnd + v_workspace + v_mgmt;
                        v_short_term_rate := ROUND(v_long_term_rate * 1.40);
                        v_kickoff := v_long_term_rate;
                        v_recruitment := 1500000;
                        v_onboarding := 10000000;
                    END IF;
                ELSIF v_role_rec.archetype = 'support' THEN
                    IF v_sen_rec.seniority = 'senior' THEN
                        v_base_salary := ROUND(6000000 * v_tier_mult);
                        v_tech_lead := ROUND(1200000 * v_tier_mult);
                        v_ai_rnd := 500000;
                        v_workspace := 500000;
                        v_mgmt := ROUND(2000000 * v_tier_mult);
                        v_long_term_rate := v_base_salary + v_tech_lead + v_ai_rnd + v_workspace + v_mgmt;
                        v_short_term_rate := ROUND(v_long_term_rate * 1.40);
                        v_kickoff := v_long_term_rate;
                        v_recruitment := 3000000;
                        v_onboarding := 12000000;
                    ELSIF v_sen_rec.seniority = 'intermediate' THEN
                        v_base_salary := ROUND(5000000 * v_tier_mult);
                        v_tech_lead := ROUND(1000000 * v_tier_mult);
                        v_ai_rnd := 500000;
                        v_workspace := 500000;
                        v_mgmt := ROUND(1500000 * v_tier_mult);
                        v_long_term_rate := v_base_salary + v_tech_lead + v_ai_rnd + v_workspace + v_mgmt;
                        v_short_term_rate := ROUND(v_long_term_rate * 1.40);
                        v_kickoff := v_long_term_rate;
                        v_recruitment := 1500000;
                        v_onboarding := 10000000;
                    ELSE -- associate
                        v_base_salary := ROUND(4000000 * v_tier_mult);
                        v_tech_lead := ROUND(800000 * v_tier_mult);
                        v_ai_rnd := 500000;
                        v_workspace := 500000;
                        v_mgmt := ROUND(1200000 * v_tier_mult);
                        v_long_term_rate := v_base_salary + v_tech_lead + v_ai_rnd + v_workspace + v_mgmt;
                        v_short_term_rate := ROUND(v_long_term_rate * 1.40);
                        v_kickoff := v_long_term_rate;
                        v_recruitment := 1500000;
                        v_onboarding := 9000000;
                    END IF;
                ELSE -- engineering default
                    IF v_sen_rec.seniority = 'senior' THEN
                        v_base_salary := ROUND(6800000 * v_tier_mult);
                        v_tech_lead := ROUND(1030000 * v_tier_mult);
                        v_ai_rnd := 500000;
                        v_workspace := 500000;
                        v_mgmt := ROUND(2720000 * v_tier_mult);
                        v_long_term_rate := v_base_salary + v_tech_lead + v_ai_rnd + v_workspace + v_mgmt;
                        v_short_term_rate := ROUND(v_long_term_rate * 1.40);
                        v_kickoff := v_long_term_rate;
                        v_recruitment := 3000000;
                        v_onboarding := 13600000;
                    ELSIF v_sen_rec.seniority = 'intermediate' THEN
                        v_base_salary := ROUND(5500000 * v_tier_mult);
                        v_tech_lead := ROUND(1675000 * v_tier_mult); -- higher tech lead oversight for intermediate
                        v_ai_rnd := 500000;
                        v_workspace := 500000;
                        v_mgmt := ROUND(1375000 * v_tier_mult);
                        v_long_term_rate := v_base_salary + v_tech_lead + v_ai_rnd + v_workspace + v_mgmt;
                        v_short_term_rate := ROUND(v_long_term_rate * 1.40);
                        v_kickoff := v_long_term_rate;
                        v_recruitment := 1500000;
                        v_onboarding := 11000000;
                    ELSE -- associate
                        v_base_salary := ROUND(4500000 * v_tier_mult);
                        v_tech_lead := ROUND(1200000 * v_tier_mult);
                        v_ai_rnd := 500000;
                        v_workspace := 500000;
                        v_mgmt := ROUND(1000000 * v_tier_mult);
                        v_long_term_rate := v_base_salary + v_tech_lead + v_ai_rnd + v_workspace + v_mgmt;
                        v_short_term_rate := ROUND(v_long_term_rate * 1.40);
                        v_kickoff := v_long_term_rate;
                        v_recruitment := 1500000;
                        v_onboarding := 10000000;
                    END IF;
                END IF;

                v_max_disc := v_role_rec.default_discount;

                -- Insert LONG-TERM item
                v_rate := v_long_term_rate;
                v_pub_rate := v_long_term_rate;
                v_floor_rate := v_pub_rate - ROUND(v_pub_rate * v_max_disc / 100.0);

                INSERT INTO rate_card_items (
                    id, company_id, rate_card_id, role, display_name, seniority, discipline, term_type,
                    amount_per_month, base_salary, tech_lead_fee, ai_rnd_fee, workspace_fee, management_fee,
                    education_requirement, experience_requirement, published_rate, max_discount_pct, floor_rate,
                    kickoff_fee, recruitment_fee, onboarding_fee, created_at, updated_at
                ) VALUES (
                    gen_random_uuid(), v_company_id, v_rc_id, v_role_rec.role, v_role_rec.display_name,
                    v_sen_rec.seniority, v_role_rec.discipline, 'long_term',
                    v_rate, v_base_salary, v_tech_lead, v_ai_rnd, v_workspace, v_mgmt,
                    v_role_rec.education, v_role_rec.experience, v_pub_rate, v_max_disc, v_floor_rate,
                    v_kickoff, v_recruitment, v_onboarding, v_now, v_now
                );

                -- Insert SHORT-TERM item
                v_rate := v_short_term_rate;
                v_pub_rate := v_short_term_rate;
                v_floor_rate := v_pub_rate - ROUND(v_pub_rate * v_max_disc / 100.0);

                INSERT INTO rate_card_items (
                    id, company_id, rate_card_id, role, display_name, seniority, discipline, term_type,
                    amount_per_month, base_salary, tech_lead_fee, ai_rnd_fee, workspace_fee, management_fee,
                    education_requirement, experience_requirement, published_rate, max_discount_pct, floor_rate,
                    kickoff_fee, recruitment_fee, onboarding_fee, created_at, updated_at
                ) VALUES (
                    gen_random_uuid(), v_company_id, v_rc_id, v_role_rec.role, v_role_rec.display_name,
                    v_sen_rec.seniority, v_role_rec.discipline, 'short_term',
                    v_rate, v_base_salary, v_tech_lead, v_ai_rnd, v_workspace, v_mgmt,
                    v_role_rec.education, v_role_rec.experience, v_pub_rate, v_max_disc, v_floor_rate,
                    v_kickoff, v_recruitment, v_onboarding, v_now, v_now
                );

                -- Insert Overtime Rate Configs (Long Term & Short Term)
                INSERT INTO overtime_rate_configs (
                    id, company_id, rate_card_id, role, seniority, term_type,
                    base_monthly_rate, hourly_divisor, workday_first_hour_multiplier,
                    workday_subsequent_multiplier, holiday_base_multiplier,
                    holiday_9th_hour_multiplier, holiday_10plus_multiplier, created_at, updated_at
                ) VALUES (
                    gen_random_uuid(), v_company_id, v_rc_id, v_role_rec.role, v_sen_rec.seniority, 'long_term',
                    v_long_term_rate, 173, 150, 200, 200, 300, 400, v_now, v_now
                );

                INSERT INTO overtime_rate_configs (
                    id, company_id, rate_card_id, role, seniority, term_type,
                    base_monthly_rate, hourly_divisor, workday_first_hour_multiplier,
                    workday_subsequent_multiplier, holiday_base_multiplier,
                    holiday_9th_hour_multiplier, holiday_10plus_multiplier, created_at, updated_at
                ) VALUES (
                    gen_random_uuid(), v_company_id, v_rc_id, v_role_rec.role, v_sen_rec.seniority, 'short_term',
                    v_short_term_rate, 173, 150, 200, 200, 300, 400, v_now, v_now
                );

            END LOOP; -- seniorities
        END LOOP; -- roles

        -- --------------------------------------------------------------------
        -- Insert Infrastructure Packages for this Rate Card (7 packages)
        -- --------------------------------------------------------------------
        -- Shared Hosting Mini
        INSERT INTO infrastructure_packages (
            id, company_id, rate_card_id, category, package_name, vcpu, ram_gb, storage_gb, amount_per_month, created_at, updated_at
        ) VALUES (
            gen_random_uuid(), v_company_id, v_rc_id, 'shared_hosting', 'Shared Hosting Mini',
            1, 2, 5, ROUND(200000 * v_tier_mult), v_now, v_now
        );

        -- Shared Hosting Small
        INSERT INTO infrastructure_packages (
            id, company_id, rate_card_id, category, package_name, vcpu, ram_gb, storage_gb, amount_per_month, created_at, updated_at
        ) VALUES (
            gen_random_uuid(), v_company_id, v_rc_id, 'shared_hosting', 'Shared Hosting Small',
            2, 3, 5, ROUND(350000 * v_tier_mult), v_now, v_now
        );

        -- Shared Hosting Medium
        INSERT INTO infrastructure_packages (
            id, company_id, rate_card_id, category, package_name, vcpu, ram_gb, storage_gb, amount_per_month, created_at, updated_at
        ) VALUES (
            gen_random_uuid(), v_company_id, v_rc_id, 'shared_hosting', 'Shared Hosting Medium',
            2, 4, 5, ROUND(600000 * v_tier_mult), v_now, v_now
        );

        -- VPS Cloud Server Mini
        INSERT INTO infrastructure_packages (
            id, company_id, rate_card_id, category, package_name, vcpu, ram_gb, storage_gb, amount_per_month, created_at, updated_at
        ) VALUES (
            gen_random_uuid(), v_company_id, v_rc_id, 'vps_cloud', 'Cloud VPS Mini',
            2, 4, 32, ROUND(800000 * v_tier_mult), v_now, v_now
        );

        -- VPS Cloud Server Small
        INSERT INTO infrastructure_packages (
            id, company_id, rate_card_id, category, package_name, vcpu, ram_gb, storage_gb, amount_per_month, created_at, updated_at
        ) VALUES (
            gen_random_uuid(), v_company_id, v_rc_id, 'vps_cloud', 'Cloud VPS Small',
            4, 4, 32, ROUND(900000 * v_tier_mult), v_now, v_now
        );

        -- VPS Cloud Server Medium
        INSERT INTO infrastructure_packages (
            id, company_id, rate_card_id, category, package_name, vcpu, ram_gb, storage_gb, amount_per_month, created_at, updated_at
        ) VALUES (
            gen_random_uuid(), v_company_id, v_rc_id, 'vps_cloud', 'Cloud VPS Medium',
            4, 8, 32, ROUND(1050000 * v_tier_mult), v_now, v_now
        );

        -- VPS Cloud Server Large
        INSERT INTO infrastructure_packages (
            id, company_id, rate_card_id, category, package_name, vcpu, ram_gb, storage_gb, amount_per_month, created_at, updated_at
        ) VALUES (
            gen_random_uuid(), v_company_id, v_rc_id, 'vps_cloud', 'Cloud VPS Large',
            6, 8, 32, ROUND(1150000 * v_tier_mult), v_now, v_now
        );

    END LOOP; -- rate cards

    RAISE NOTICE 'Venturo Master Rate Cards Seeding Completed Successfully!';
END $$;
