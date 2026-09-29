ALTER TABLE quotation_revisions
    DROP CONSTRAINT IF EXISTS chk_prompt_state_version_non_negative;

ALTER TABLE quotation_revisions
    DROP COLUMN IF EXISTS prompt_section_hashes,
    DROP COLUMN IF EXISTS prompt_state_version,
    DROP COLUMN IF EXISTS prompt_state;
