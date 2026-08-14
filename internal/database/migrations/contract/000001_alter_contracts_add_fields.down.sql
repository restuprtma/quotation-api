ALTER TABLE contracts
DROP COLUMN IF EXISTS client_snapshot,
DROP COLUMN IF EXISTS scope_data,
DROP COLUMN IF EXISTS deliverables_data,
DROP COLUMN IF EXISTS commercial_terms,
DROP COLUMN IF EXISTS payment_terms,
DROP COLUMN IF EXISTS timeline_data;
