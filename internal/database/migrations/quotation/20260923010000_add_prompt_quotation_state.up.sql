ALTER TABLE quotation_revisions
    ADD COLUMN prompt_state JSONB NULL,
    ADD COLUMN prompt_state_version BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN prompt_section_hashes JSONB NULL;

ALTER TABLE quotation_revisions
    ADD CONSTRAINT chk_prompt_state_version_non_negative
    CHECK (prompt_state_version >= 0);
