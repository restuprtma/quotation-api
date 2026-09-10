ALTER TABLE quotations DROP CONSTRAINT IF EXISTS chk_quotations_commercial_scheme;
ALTER TABLE quotations ADD CONSTRAINT chk_quotations_commercial_scheme
CHECK (commercial_scheme IN ('dedicated_squad', 'subscription_saas', 'white_label', 'standalone_maintenance', 'custom_development', 'kiosk_queue_system'));

CREATE UNIQUE INDEX IF NOT EXISTS uq_quotations_company_number_active
ON quotations (company_id, quotation_number)
WHERE deleted_at IS NULL;
