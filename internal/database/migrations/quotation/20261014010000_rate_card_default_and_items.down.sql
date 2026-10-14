DROP INDEX IF EXISTS uq_rate_cards_company_default;
ALTER TABLE rate_cards DROP COLUMN IF EXISTS is_default;
