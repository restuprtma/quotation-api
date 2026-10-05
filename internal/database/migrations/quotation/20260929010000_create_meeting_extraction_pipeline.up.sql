CREATE TABLE quotation_extraction_runs (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id uuid NOT NULL REFERENCES core.companies(id),
    quotation_id uuid NOT NULL REFERENCES quotations(id) ON DELETE CASCADE,
    revision_id uuid NOT NULL REFERENCES quotation_revisions(id) ON DELETE CASCADE,
    status varchar(32) NOT NULL CONSTRAINT chk_extraction_run_status CHECK (status IN ('queued','running','waiting_rate_limit','input_required','needs_review','failed','succeeded','cancelled')),
    current_phase varchar(32),
    failed_phase varchar(32),
    selected_scheme varchar(64) CONSTRAINT chk_extraction_selected_scheme CHECK (selected_scheme IS NULL OR selected_scheme IN ('dedicated_squad','subscription_saas','white_label','standalone_maintenance','custom_development','kiosk_queue_system','talent_outsourcing','cloud_infrastructure')),
    scheme_source varchar(16) CONSTRAINT chk_extraction_scheme_source CHECK (scheme_source IN ('manual','extracted') OR scheme_source IS NULL),
    error_code varchar(64),
    retryable boolean NOT NULL DEFAULT false,
    retry_after timestamptz,
    calls_used smallint NOT NULL DEFAULT 0 CONSTRAINT chk_extraction_calls_used CHECK (calls_used >= 0),
    calls_max smallint NOT NULL DEFAULT 8 CONSTRAINT chk_extraction_calls_max CHECK (calls_max BETWEEN 1 AND 10),
    input_tokens bigint NOT NULL DEFAULT 0 CONSTRAINT chk_extraction_input_tokens CHECK (input_tokens >= 0),
    output_tokens bigint NOT NULL DEFAULT 0 CONSTRAINT chk_extraction_output_tokens CHECK (output_tokens >= 0),
    note_hash char(64) NOT NULL CONSTRAINT chk_extraction_note_hash CHECK (note_hash ~ '^[0-9a-f]{64}$'),
    pipeline_version varchar(64) NOT NULL,
    lease_owner varchar(128),
    lease_expires_at timestamptz,
    fence bigint NOT NULL DEFAULT 0 CONSTRAINT chk_extraction_fence CHECK (fence >= 0),
    started_at timestamptz,
    completed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX uq_extraction_one_active_per_quotation
    ON quotation_extraction_runs (quotation_id)
    WHERE status IN ('queued','running','waiting_rate_limit','input_required');

CREATE INDEX ix_extraction_claim
    ON quotation_extraction_runs (status, retry_after, lease_expires_at, created_at);

CREATE TABLE quotation_extraction_phases (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    run_id uuid NOT NULL REFERENCES quotation_extraction_runs(id) ON DELETE CASCADE,
    phase_key varchar(32) NOT NULL,
    ordinal smallint NOT NULL CONSTRAINT chk_extraction_phase_ordinal CHECK (ordinal > 0),
    status varchar(16) NOT NULL CONSTRAINT chk_extraction_phase_status CHECK (status IN ('pending','running','succeeded','failed','skipped')),
    attempt smallint NOT NULL DEFAULT 0 CONSTRAINT chk_extraction_phase_attempt CHECK (attempt >= 0),
    transient_retries smallint NOT NULL DEFAULT 0 CONSTRAINT chk_extraction_phase_transient_retries CHECK (transient_retries >= 0),
    repair_retries smallint NOT NULL DEFAULT 0 CONSTRAINT chk_extraction_phase_repair_retries CHECK (repair_retries >= 0),
    input_fingerprint char(64),
    result_json jsonb CONSTRAINT chk_extraction_phase_result_size CHECK (result_json IS NULL OR octet_length(result_json::text) <= 65536),
    evidence_refs jsonb NOT NULL DEFAULT '[]'::jsonb CONSTRAINT chk_extraction_phase_evidence_refs CHECK (jsonb_typeof(evidence_refs) = 'array' AND jsonb_array_length(evidence_refs) <= 128),
    warnings jsonb NOT NULL DEFAULT '[]'::jsonb CONSTRAINT chk_extraction_phase_warnings CHECK (jsonb_typeof(warnings) = 'array' AND jsonb_array_length(warnings) <= 128),
    error_code varchar(64),
    retryable boolean NOT NULL DEFAULT false,
    provider varchar(32),
    model varchar(128),
    duration_ms bigint CONSTRAINT chk_extraction_phase_duration CHECK (duration_ms IS NULL OR duration_ms >= 0),
    input_tokens bigint NOT NULL DEFAULT 0 CONSTRAINT chk_extraction_phase_input_tokens CHECK (input_tokens >= 0),
    output_tokens bigint NOT NULL DEFAULT 0 CONSTRAINT chk_extraction_phase_output_tokens CHECK (output_tokens >= 0),
    fence bigint NOT NULL DEFAULT 0 CONSTRAINT chk_extraction_phase_fence CHECK (fence >= 0),
    started_at timestamptz,
    completed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uq_extraction_phase_per_run UNIQUE (run_id, phase_key)
);

CREATE TABLE quotation_extraction_provider_state (
    provider varchar(32) NOT NULL,
    model varchar(128) NOT NULL,
    usage_date date NOT NULL,
    calls_used bigint NOT NULL DEFAULT 0 CONSTRAINT chk_extraction_provider_calls CHECK (calls_used >= 0),
    input_tokens bigint NOT NULL DEFAULT 0 CONSTRAINT chk_extraction_provider_input_tokens CHECK (input_tokens >= 0),
    output_tokens bigint NOT NULL DEFAULT 0 CONSTRAINT chk_extraction_provider_output_tokens CHECK (output_tokens >= 0),
    consecutive_transient_failures integer NOT NULL DEFAULT 0 CONSTRAINT chk_extraction_provider_failures CHECK (consecutive_transient_failures >= 0),
    circuit_open_until timestamptz,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (provider, model, usage_date)
);
