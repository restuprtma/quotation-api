# Backend Data Model

Core quotation tables:

- `quotations`: tenant, client, subject, project type, status, current revision.
- `quotation_revisions`: immutable shared version, dates, snapshots, narrative JSON, product/project detail JSON, blueprint JSON, signature fields.
- `quotation_pricing_sections`: product package/service/project phase grouping.
- `quotation_pricing_items`: unit/resource/fixed pricing inputs and server-calculated line total.
- `quotation_public_artifacts`: one UUID token per revision and artifact type: quotation, blueprint, detail.
- `contracts`: generated only from signed quotation revision.

Every tenant-owned table has `company_id UUID NOT NULL`, UTC timestamps, foreign-key indexes, and UUID primary keys.
