ALTER TABLE rate_cards ADD COLUMN IF NOT EXISTS is_default BOOLEAN NOT NULL DEFAULT FALSE;

CREATE UNIQUE INDEX IF NOT EXISTS uq_rate_cards_company_default
ON rate_cards (company_id)
WHERE is_default = TRUE;
