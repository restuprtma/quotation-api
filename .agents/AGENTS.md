# Agent Guide - quotation-api

## Token-efficient reading

Before coding, read this file and the short `PRD.md`. Do **not** read every product document.

Read only the relevant detail:

- Database/schema: `docs/product/data-model.md`.
- Pricing/calculation: `docs/product/pricing-model.md`.
- AI provider/JSON validation: `docs/product/ai-contract.md`.
- Public artifact routes/security: `docs/product/public-artifacts.md` and `docs/product/security-rules.md`.
- Contract: `docs/product/contract-generator.md`.

Read the nearest existing module implementation before creating a new backend module.

## Project scope

This backend supports the Quotation & Contract Generator. Core auth/client modules remain platform dependencies. Quotation business logic must support product-based pricing, project-based pricing, structured AI bundles, three public artifacts, signatures, and contracts.

## Commands

- Go 1.25, Gin, PostgreSQL, pgxpool.
- `make dev` starts development with hot reload.
- `make run` runs without hot reload.
- `make migrate-up` applies migrations.
- `make migrate-version` checks migration state.
- `go test ./...` runs all tests.
- `go build ./...` checks compilation.
- `gofmt -w <paths>` formats Go files.

## Non-negotiable rules

- All internal tenant queries filter by `company_id`.
- Public routes do not use JWT/company context, but require UUID artifact tokens and artifact-type validation.
- Public DTOs must not expose tenant ID, creator ID, raw AI response, internal notes, or signed IP.
- Server recalculates all prices using integer arithmetic.
- AI cannot decide or overwrite prices, rates, quantity, employee count, duration, discount, tax, or totals.
- AI output is versioned JSON and must be validated before persistence.
- API keys belong only in backend environment variables.
- Create a new migration; do not rewrite migrations already applied.
- Signature data is size-limited PNG data URL; signed rows cannot be overwritten.

## Architecture

Feature modules use:

```text
internal/modules/<domain>/<feature>/
├── domain
├── dto
├── handler
├── repository
├── service
└── main.<feature>.go
```

Quotation routes are registered in `internal/modules/quotation/main.quotation.go` and mounted by `internal/router/router.go`.

## Workflow

1. Read only the short `D:\venturo\Quotation-generator\todo.md` index; do not read the whole roadmap.
2. Open the linked phase file, for example `D:\venturo\Quotation-generator\docs\roadmap\phase-02-domain-pricing.md`.
3. Read only the product/detail documents listed inside that phase file.
4. Inspect current migrations, DTOs, handlers, repository, and service before editing.
5. Make a new migration for schema changes.
6. Run `gofmt`, focused tests, `go test ./...`, and `go build ./...` as appropriate.
7. Update the phase file checkbox and root `todo.md` status only after verification.
