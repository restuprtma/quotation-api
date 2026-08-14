# Backend Product Contract - Quotation & Contract Generator v2

This is the short backend contract. Read task-specific details from `docs/product/README.md`; do not read every document for a small task.

## Backend Responsibilities

- Multi-tenant quotation CRUD and revisioning.
- Server-authoritative pricing calculation.
- Structured AI provider interface and bundle validation.
- Three public artifact endpoints.
- Simple quotation/contract signature storage.
- Contract generation from signed quotation revisions.

## Core Rules

- Every internal tenant query filters by `company_id`.
- Every tenant table uses UUID IDs and UTC timestamps.
- Money uses integer Rupiah, never float.
- AI never owns prices or totals.
- AI returns validated versioned JSON, never executable HTML/CSS.
- Public tokens are random UUIDs tied to one revision and artifact type.
- Public DTOs exclude tenant ID, creator ID, raw AI response, internal notes, and signed IP.
- Signature stores bounded PNG data, server timestamp, and server-observed IP; signed records cannot be overwritten.

## Pricing

- `unit`: `unit_price * quantity`.
- `resource`: `unit_price * employee_count * duration`.
- `fixed`: `unit_price`.
- Billing: `one_time`, `monthly`, `yearly`, `custom`.
- Server recalculates every total.

Details: `docs/product/pricing-model.md` and `docs/product/data-model.md`.

## AI Bundle

Response keys:

- `schema_version`
- `narrative`
- `product_or_project_detail`
- `blueprint`
- `pricing_labels`

Providers may be Mock, Ollama, Gemini, OpenRouter, or Groq. Credentials remain backend-only. Details: `docs/product/ai-contract.md`.

## Public API

```text
GET  /p/q/:artifact_token
POST /p/q/:artifact_token/sign
GET  /p/b/:artifact_token
GET  /p/d/:artifact_token
```

Internal sharing creates three artifact tokens:

```text
POST /quotation/v1/quotations/:id/share
GET  /quotation/v1/quotations/:id/artifacts
```

Details: `docs/product/public-artifacts.md` and `docs/product/security-rules.md`.

## Contract

Only `signed` quotations create contracts. Contract lifecycle is `draft -> shared -> signed`. Details: `docs/product/contract-generator.md`.
