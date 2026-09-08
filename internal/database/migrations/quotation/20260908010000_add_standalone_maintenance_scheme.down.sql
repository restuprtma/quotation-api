ALTER TABLE quotations DROP CONSTRAINT IF EXISTS chk_quotations_commercial_scheme;
ALTER TABLE quotations
    ADD CONSTRAINT chk_quotations_commercial_scheme
    CHECK (commercial_scheme IN ('dedicated_squad', 'subscription_saas', 'white_label'));
