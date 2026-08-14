ALTER TABLE contracts
ADD COLUMN client_snapshot JSONB,
ADD COLUMN scope_data TEXT,
ADD COLUMN deliverables_data TEXT,
ADD COLUMN commercial_terms TEXT,
ADD COLUMN payment_terms TEXT,
ADD COLUMN timeline_data TEXT;
