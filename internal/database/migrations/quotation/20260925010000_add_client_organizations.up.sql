CREATE TABLE quotation_client_organizations (
    company_id UUID NOT NULL,
    id UUID NOT NULL DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL,
    normalized_name VARCHAR(255) NOT NULL,
    created_by UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ,
    PRIMARY KEY (company_id, id)
);

CREATE UNIQUE INDEX uq_quotation_client_org_active_name
    ON quotation_client_organizations (company_id, normalized_name)
    WHERE deleted_at IS NULL;

INSERT INTO quotation_client_organizations (company_id, id, name, normalized_name, created_by)
SELECT DISTINCT
    q.company_id,
    q.client_id,
    REGEXP_REPLACE(BTRIM(c.name), '\s+', ' ', 'g'),
    LOWER(REGEXP_REPLACE(BTRIM(c.name), '\s+', ' ', 'g')),
    q.created_by
FROM quotations q
JOIN core.clients c ON c.id = q.client_id
WHERE q.deleted_at IS NULL
ON CONFLICT DO NOTHING;
