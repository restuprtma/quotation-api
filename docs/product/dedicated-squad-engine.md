# Dedicated Squad quotation engine

Status: rollout contract for the deterministic Dedicated Squad calculator, rules, timeline, renderer, and generation pipeline.

This document is the backend product contract. The implementation and tests remain the source of truth for behaviour; update this document when a public field, rule, error code, snapshot, or renderer invariant changes.

## Scope and source of truth

Dedicated Squad is selected with <code>commercial_scheme: "dedicated_squad"</code> and a <code>dedicated_squad</code> object. It supports four quotation models:

| Model | Product name | Commercial intent |
| --- | --- | --- |
| <code>1A</code> | SOW Large | Project subtotal at least Rp150.000.000, with hiring preparation and security work. |
| <code>1B</code> | SOW Small | Project subtotal below Rp150.000.000, with tiered delivery and equal installments. |
| <code>1C</code> | Staff Augmentation | Monthly capacity, engineering kick-off, and an explicit 50% or 100% kick-off discount. |
| <code>1D</code> | Sprint | Short, weekday-only delivery with one settlement. |

The four benchmark fixtures are historical-sheet references, not visual goldens. They use the active <code>VENTURO-STD-2026</code> master card and deterministic current rules. A historical sheet can therefore disagree on a rate, payment date, phase label, or grouping. Such variance belongs in fixture metadata and review notes; it must not be silently corrected by changing the current calculator. The fixtures explicitly carry <code>visual_parity: "not_claimed"</code> until a human review has approved exported references.

The current master card is seeded by <code>internal/database/seeders/quotation/001_venturo_master_rate_cards.sql</code>. Production resolution is tenant-scoped, requires an active card, and matches role, seniority, and term type case-insensitively. Benchmarks use an in-memory lookup with the same values so that they have no database dependency.

## Request contract

Routes are mounted under <code>/quotation/v1</code>, require the normal authenticated company context, and accept the standard response envelope:

~~~http
POST /quotation/v1/quotations/preview
POST /quotation/v1/quotations/generate
POST /quotation/v1/quotations/:id/regenerate
GET  /quotation/v1/quotations/:id/generation-status
~~~

<code>preview</code> calculates only the server-authoritative commercial view. It does not persist or call an AI provider. <code>generate</code> creates a durable draft before any provider call and returns the quotation ID. <code>generation-status</code> is authoritative while generation is pending. Date fields accept either an ISO calendar date (<code>2026-07-14</code>) or RFC3339 (<code>2026-07-14T00:00:00Z</code>); persisted snapshots use the unambiguous RFC3339 form.

The common envelope fields are:

~~~json
{
  "idempotency_key": "00000000-0000-4000-8000-000000000001",
  "skill_id": "00000000-0000-4000-8000-000000000002",
  "client_name": "Benchmark Client",
  "client_company_name": "PT Benchmark",
  "subject": "Dedicated Squad quotation",
  "solution_type": "dedicated_squad",
  "commercial_scheme": "dedicated_squad",
  "rate_card_id": "00000000-0000-4000-8000-000000000003",
  "dedicated_squad": {}
}
~~~

The UUIDs above are placeholders for IDs from the authenticated tenant. <code>rate_card_code</code> may be included for display, but <code>rate_card_id</code> is required for dedicated generation. Legacy allocation, budget, and payment-term fields are not used to calculate a dedicated quotation once <code>dedicated_squad</code> is present.

### 1A — SOW Large

~~~json
{
  "idempotency_key": "00000000-0000-4000-8000-000000000101",
  "skill_id": "00000000-0000-4000-8000-000000000002",
  "client_name": "Benchmark Client",
  "client_company_name": "PT Benchmark 250JT",
  "subject": "Marketplace portal — SOW Large",
  "solution_type": "dedicated_squad",
  "commercial_scheme": "dedicated_squad",
  "rate_card_id": "00000000-0000-4000-8000-000000000003",
  "dedicated_squad": {
    "model": "1A",
    "requirement": "Build a marketplace portal with registration, reservation, payments, analytics, and access control.",
    "development_squad": [
      { "role": "ui_ux_designer", "seniority": "senior", "headcount_quarters": 4, "period_quarters": 4 },
      { "role": "business_analyst", "seniority": "senior", "headcount_quarters": 4, "period_quarters": 4 },
      { "role": "backend_programmer", "seniority": "senior", "headcount_quarters": 4, "period_quarters": 12 },
      { "role": "backend_programmer", "seniority": "intermediate", "headcount_quarters": 4, "period_quarters": 12 },
      { "role": "mobile_developer", "seniority": "senior", "headcount_quarters": 4, "period_quarters": 12 },
      { "role": "functional_tester", "seniority": "intermediate", "headcount_quarters": 4, "period_quarters": 12 }
    ],
    "maintenance_enabled": true,
    "maintenance_squad": [
      { "role": "devops", "seniority": "senior", "headcount_quarters": 4, "period_quarters": 4 },
      { "role": "data_analyst", "seniority": "intermediate", "headcount_quarters": 4, "period_quarters": 4 }
    ],
    "payment_count": 6,
    "payment_anchor_date": "2026-09-01T00:00:00Z",
    "timeline": { "mode": "relative" }
  }
}
~~~

For the full benchmark input, use <code>internal/modules/quotation/service/testdata/dedicated_squad/sow-250jt.json</code>. With the fixture's six development lines and two maintenance lines, current-card expected values are development Rp161.500.000, maintenance Rp35.070.000, hiring preparation gross Rp90.400.000, preparation net Rp45.200.000, and total investment Rp241.770.000. The rules include K6, penetration testing, timeline, and maintenance.

Preview response (abbreviated only where arrays are unchanged from the calculation contract):

~~~json
{
  "data": {
    "calculation": {
      "model": "1A",
      "development_subtotal": 161500000,
      "maintenance_subtotal": 35070000,
      "preparation": { "kind": "hiring", "gross": 90400000, "discount": 45200000, "net": 45200000 },
      "total": 286970000,
      "discount": 45200000,
      "total_investment": 241770000,
      "payments": "PaymentRow[]"
    },
    "visible_sections": [
      { "number": 1, "code": "development", "title": "Biaya Development" },
      { "number": 2, "code": "maintenance", "title": "Biaya Maintenance" },
      { "number": 3, "code": "timeline", "title": "Lingkup Pekerjaan & Timeline" },
      { "number": 4, "code": "payment", "title": "Pembayaran" },
      { "number": 5, "code": "legal", "title": "Batasan dan Pengecualian" }
    ],
    "include_k6": true,
    "include_pentest": true
  },
  "message": "Dedicated squad preview calculated successfully"
}
~~~

### 1B — SOW Small

~~~json
{
  "idempotency_key": "00000000-0000-4000-8000-000000000102",
  "skill_id": "00000000-0000-4000-8000-000000000002",
  "client_name": "Benchmark Client",
  "client_company_name": "PT Benchmark 50JT",
  "subject": "Commerce portal — SOW Small",
  "solution_type": "dedicated_squad",
  "commercial_scheme": "dedicated_squad",
  "rate_card_id": "00000000-0000-4000-8000-000000000003",
  "dedicated_squad": {
    "model": "1B",
    "requirement": "Build a compact commerce portal with catalog, checkout, and order tracking.",
    "development_squad": [
      { "role": "ui_ux_designer", "seniority": "intermediate", "headcount_quarters": 4, "period_quarters": 4 },
      { "role": "backend_programmer", "seniority": "intermediate", "headcount_quarters": 4, "period_quarters": 4 }
    ],
    "maintenance_enabled": false,
    "payment_count": 3,
    "payment_anchor_date": "2026-06-17T00:00:00Z",
    "timeline": { "mode": "relative" }
  }
}
~~~

For the full benchmark input, use <code>sow-50jt.json</code>. Its current-card expected development subtotal and total investment are Rp66.290.000. The project is tier 50: timeline is visible, maintenance is hidden, and K6/penetration testing are hidden. The three installments are Rp22.096.666, Rp22.096.666, and Rp22.096.668; the remainder is deliberately assigned to the final row.

~~~json
{
  "data": {
    "calculation": {
      "model": "1B",
      "development_subtotal": 66290000,
      "maintenance_subtotal": 0,
      "preparation": { "kind": "none", "gross": 0, "discount": 0, "net": 0 },
      "total": 66290000,
      "discount": 0,
      "total_investment": 66290000,
      "payments": [
        { "work_period": "Hiring & Kick off", "terms": "Down Payment", "nominal": 22096666 },
        { "work_period": "Bulan 2", "terms": "Termin 2", "nominal": 22096666 },
        { "work_period": "Bulan 3", "terms": "Termin 3", "nominal": 22096668 }
      ]
    },
    "visible_sections": [
      { "number": 1, "code": "development", "title": "Biaya Development" },
      { "number": 2, "code": "timeline", "title": "Lingkup Pekerjaan & Timeline" },
      { "number": 3, "code": "payment", "title": "Pembayaran" },
      { "number": 4, "code": "legal", "title": "Batasan dan Pengecualian" }
    ],
    "include_k6": false,
    "include_pentest": false
  },
  "message": "Dedicated squad preview calculated successfully"
}
~~~

### 1C — Staff Augmentation

~~~json
{
  "idempotency_key": "00000000-0000-4000-8000-000000000103",
  "skill_id": "00000000-0000-4000-8000-000000000002",
  "client_name": "Benchmark Client",
  "client_company_name": "Kopi Studio",
  "subject": "Dedicated engineering capacity",
  "solution_type": "dedicated_squad",
  "commercial_scheme": "dedicated_squad",
  "rate_card_id": "00000000-0000-4000-8000-000000000003",
  "dedicated_squad": {
    "model": "1C",
    "requirement": "Provide dedicated engineering capacity for the client's product backlog.",
    "development_squad": [
      { "role": "mobile_developer", "seniority": "senior", "headcount_quarters": 4, "period_quarters": 24 },
      { "role": "backend_programmer", "seniority": "intermediate", "headcount_quarters": 2, "period_quarters": 4 }
    ],
    "maintenance_enabled": false,
    "kickoff_discount_bps": 5000,
    "payment_count": 6,
    "payment_anchor_date": "2026-08-17T00:00:00Z",
    "timeline": { "mode": "" }
  }
}
~~~

For the full benchmark input, use <code>kopi-studio.json</code>. With a 50% kick-off discount, current-card expected development subtotal is Rp74.075.000, kick-off gross is Rp16.325.000, kick-off net is Rp8.162.500, and total investment is Rp82.237.500. Staff augmentation has no generated timeline, K6, or penetration-test section. <code>payment_count</code> must equal the longest resource period in months (six in this example).

~~~json
{
  "data": {
    "calculation": {
      "model": "1C",
      "development_subtotal": 74075000,
      "maintenance_subtotal": 0,
      "preparation": { "kind": "engineering_kickoff", "gross": 16325000, "discount": 8162500, "net": 8162500 },
      "total": 90400000,
      "discount": 8162500,
      "total_investment": 82237500,
      "payments": "PaymentRow[] (kick-off plus six monthly rows)"
    },
    "visible_sections": [
      { "number": 1, "code": "development", "title": "Biaya Development" },
      { "number": 2, "code": "payment", "title": "Pembayaran" },
      { "number": 3, "code": "legal", "title": "Batasan dan Pengecualian" }
    ],
    "include_k6": false,
    "include_pentest": false
  },
  "message": "Dedicated squad preview calculated successfully"
}
~~~

### 1D — Sprint

~~~json
{
  "idempotency_key": "00000000-0000-4000-8000-000000000104",
  "skill_id": "00000000-0000-4000-8000-000000000002",
  "client_name": "Benchmark Client",
  "client_company_name": "Cuit",
  "subject": "Checkout and payment callback sprint",
  "solution_type": "dedicated_squad",
  "commercial_scheme": "dedicated_squad",
  "rate_card_id": "00000000-0000-4000-8000-000000000003",
  "dedicated_squad": {
    "model": "1D",
    "requirement": "- Adjust responsive checkout layout\n- Fix address validation\n- Integrate payment callback webhook\n- Verify release APK",
    "development_squad": [
      { "role": "mobile_developer", "seniority": "senior", "headcount_quarters": 4, "workday_count": 4 }
    ],
    "maintenance_enabled": false,
    "payment_count": 1,
    "payment_anchor_date": "2026-07-14T00:00:00Z",
    "timeline": {
      "mode": "sprint",
      "start_date": "2026-07-14T00:00:00Z",
      "sprint_end_date": "2026-07-17T00:00:00Z"
    }
  }
}
~~~

For the full benchmark input, use <code>cuit.json</code>. The current short-term mobile rate of Rp16.170.000 divided by 22 working days floors to Rp735.000/day, so four workdays produce Rp2.940.000. A sprint has one <code>Penyelesaian pekerjaan</code> / <code>Pelunasan</code> payment, invoices at sprint end, and no AI module extraction.

~~~json
{
  "data": {
    "calculation": {
      "model": "1D",
      "development_subtotal": 2940000,
      "maintenance_subtotal": 0,
      "preparation": { "kind": "none", "gross": 0, "discount": 0, "net": 0 },
      "total": 2940000,
      "discount": 0,
      "total_investment": 2940000,
      "payments": [
        { "work_period": "Penyelesaian pekerjaan", "terms": "Pelunasan", "nominal": 2940000 }
      ]
    },
    "visible_sections": [
      { "number": 1, "code": "development", "title": "Biaya Development" },
      { "number": 2, "code": "timeline", "title": "Lingkup Pekerjaan & Timeline" },
      { "number": 3, "code": "payment", "title": "Pembayaran" },
      { "number": 4, "code": "legal", "title": "Batasan dan Pengecualian" }
    ],
    "include_k6": false,
    "include_pentest": false
  },
  "message": "Dedicated squad preview calculated successfully"
}
~~~

For all four models, a successful generate response is <code>201 Created</code>:

~~~json
{
  "data": { "quotation_id": "00000000-0000-4000-8000-000000000999" },
  "message": "Quotation generated successfully"
}
~~~

Poll <code>GET /quotations/:id/generation-status</code>; do not infer completion from the generate response. A completed status includes the generated HTML and <code>content_version</code>. If the pipeline fails after draft creation, the generate/regenerate response carries the safe error code and the later status read shows <code>failed</code> without exposing an internal failure reason; the draft remains retryable.

## Rule matrix

The calculator produces the development subtotal first. Rules then select sections, preparation, discounts, security work, and payment mode.

| Rule | 1A | 1B | 1C | 1D |
| --- | --- | --- | --- | --- |
| Qualification | Development subtotal <code>>= Rp150m</code> | Development subtotal <code>&lt; Rp150m</code> | <code>payment_count</code> equals longest period in months | <code>payment_count = 1</code>, sprint dates valid |
| Preparation | Hiring | None | Engineering kick-off | None |
| Preparation discount | 50% of hiring gross | None | Request must be 50% or 100% (<code>5000</code>/<code>10000</code> bps) | None |
| Payment kind | Net-preparation down payment, then installments | Equal installments | Monthly, after kick-off down payment | Single settlement |
| Timeline | Included | Included | Omitted | Included sprint task list |
| Maintenance | Input-controlled; only model allowed to provide it | Never | Never | Never |
| K6 / penetration test | Both included | Only tier 100 includes both | Neither | Neither |
| Project tier | Not used | <code>&lt;50m</code> = 20; <code>50m–&lt;100m</code> = 50; <code>100m–&lt;150m</code> = 100 | Not used | Not used |
| Timeline mode | <code>relative</code> or <code>calendar</code> | <code>relative</code> or <code>calendar</code> | Empty / unsupported | <code>sprint</code> |

The rules are evaluated after the current rate-card calculation. If a 1A input is below the threshold, or a 1B input is at/above it, validation fails; the engine does not automatically switch models.

## Deterministic arithmetic and dates

- Non-sprint line totals use integer quarter units: <code>amount_per_month * headcount_quarters * period_quarters / 16</code>. Four quarters represents one employee-month. The selected term is long-term when <code>period_quarters >= 12</code> (otherwise short-term) for 1A/1B; 1C always uses long-term.
- Sprint uses the short-term monthly rate divided by 22, with integer floor, as the daily rate. The line total is <code>daily_rate * headcount_quarters * workday_count / 4</code>.
- Preparation gross is the per-head preparation fee multiplied by headcount. Discount is integer <code>gross * basis_points / 10000</code>; total investment is <code>total - discount</code>.
- Equal-installment division is integer division. Any remainder is added to the final payment row, so the sum of rows must equal total investment exactly. Never round each row independently.
- All money is integer IDR (<code>int64</code>); no floating-point arithmetic is permitted. Overflow or a missing rate item is an allocation failure.
- Due dates are invoice date plus 14 calendar days (<code>H+14</code>), not fourteen business days. The sprint invoice date is the sprint end date. Other invoice dates follow the selected anchor/month schedule.
- The timeline generator excludes no public holidays. <code>calendar</code> counts calendar days; sprint weekdays are Monday–Friday, inclusive. It emits the canonical phase/deliverable catalog and the renderer flattens module names and sub-features into timeline deliverables.

The renderer is intentionally deterministic around the official structure: root <code>article.quotation-document</code>, fixed heading <code>PENAWARAN LAYANAN DEDICATED PROGRAMMER SQUAD</code>, sequential section numbers, cost/payment column headers, four legal exclusions, and two signature columns. Current benchmark assertions cover these DOM invariants. They do not constitute a pixel or visual-parity claim.

## AI boundary and safety

The rate card, role, seniority, headcount, periods, money, rules, payment rows, and timeline are server-authoritative. AI may extract modules/sub-features for 1A–1C and compose narrative content; it cannot alter commercial values or add unsanctioned sections. Model 1D skips the provider and uses requirement lines as sprint deliverables.

Handlers and <code>GenerationError.Error()</code> expose safe static codes. Do not log or return API keys, provider credentials, prompts, raw provider responses, token counts, stack traces, or unredacted client data. Test fixtures and screenshots must contain no secrets or raw provider logs. Provider diagnostics, if needed for operations, belong in restricted/redacted telemetry and are outside the quotation response contract.

## Persistence, snapshots, and regeneration

Generation proceeds in this order:

1. Validate the request and resolve the tenant's skill, active rate card, rate items, legal template, client, and quotation number.
2. Persist a queued quotation/revision and pending draft before contacting an AI provider.
3. Calculate money, evaluate rules, extract modules when applicable, generate the timeline, render HTML, sanitize it, and validate document integrity.
4. Commit HTML and all dedicated snapshots atomically.

The snapshot migration adds nullable JSONB columns to <code>quotation_revisions</code>:

| Snapshot | Contents | Regeneration use |
| --- | --- | --- |
| <code>calculation_snapshot</code> | Current-card lines, preparation, totals, and payments | Reused; rates are not silently recalculated from today's card. |
| <code>module_snapshot</code> | Extracted modules/sub-features | Replaced after successful 1A–1C regeneration. |
| <code>timeline_snapshot</code> | Generated timeline rows | Replaced after successful regeneration. |

Regeneration is limited to a tenant-owned active draft. It consumes at most five attempts per draft per hour before provider work, reuses the stored calculation snapshot, re-evaluates rules, re-extracts modules except for 1D, regenerates the timeline, and atomically replaces HTML and snapshots. <code>content_version</code> increments only after a successful commit. A provider, sanitization, or persistence failure leaves the previous HTML and snapshots unchanged and the draft retryable. Internal snapshots are not exposed by public quotation detail/status responses.

The migration is <code>20260904020000_dedicated_squad_snapshots</code>. In a disposable QA database, migrate with <code>make migrate-up</code>; verify with <code>make migrate-version</code>; roll back only the latest quotation migration with <code>make migrate-down MODULE=quotation</code>. The down migration drops the three snapshot columns. Treat that as destructive to regeneration metadata: existing HTML may remain, but regeneration cannot use a missing calculation snapshot. Never perform a migration rollback in production without an approved backup and release decision.

## Error contract

The handler returns a static message and, where applicable, <code>errors.detail: [code]</code>. The table is the supported operational vocabulary; internal causes are intentionally not returned.

| Code | Usual HTTP | Meaning / client action |
| --- | ---: | --- |
| <code>settings_incomplete</code> | 422 | Skill/provider or legal configuration is incomplete; fix tenant configuration. |
| <code>invalid_client</code> | 422 | Client cannot be resolved in the tenant; select a valid client. |
| <code>invalid_rate_card</code> | 422 | Card is missing, inactive, foreign, or has no usable items. |
| <code>rate_item_missing</code> | 422 | Role/seniority/term has no card item; change the allocation or card. |
| <code>budget_unfeasible</code> | 422 | A legacy/AI allocation cannot fit its budget; dedicated deterministic inputs should be corrected. |
| <code>allocation_invalid</code> | 422 | Model, threshold, line, timeline, or payment-count validation failed. |
| <code>provider_invalid_credential</code> | 422 | Provider credential is invalid; rotate/configure it server-side. |
| <code>provider_rate_limited</code> | 422 | Provider rejected the request rate; retry according to operations policy. |
| <code>provider_timeout</code> | 422 | Provider did not answer in time; retry the draft. |
| <code>unsafe_html</code> | 422 | Rendered or edited content failed sanitization/integrity checks. |
| <code>generation_failed</code> | 422 | Safe catch-all for generation, snapshot, or persistence failure. |
| <code>regenerate_rate_limited</code> | 429 | Draft exceeded five regeneration attempts in one hour. |
| <code>revision_conflict</code> | 409 | Draft is no longer active or revision/content version is stale. |

Malformed JSON is <code>400</code>; a missing/foreign quotation on regeneration is <code>404</code>; no existence distinction is made across tenants. A client should poll status after a <code>201</code> and should not display an internal error string as a diagnosis.

## Verification and references

Run from <code>quotation-api</code>:

~~~text
go test ./internal/modules/quotation/service -run DedicatedSquadBenchmark -count=1
go test ./... -count=1
~~~

The benchmark test compares every financial, structural, timeline, and DOM field in:

- <code>internal/modules/quotation/service/testdata/dedicated_squad/sow-250jt.json</code>
- <code>internal/modules/quotation/service/testdata/dedicated_squad/sow-50jt.json</code>
- <code>internal/modules/quotation/service/testdata/dedicated_squad/kopi-studio.json</code>
- <code>internal/modules/quotation/service/testdata/dedicated_squad/cuit.json</code>

The root QA workflow, including historical variance recording and human visual sign-off, is in [docs/qa/dedicated-squad-benchmark-checklist.md](../../../docs/qa/dedicated-squad-benchmark-checklist.md).
