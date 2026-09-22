# Blueprint Provider Error Recovery Runbook

This document defines the operational procedures, error classifications, and recovery workflows for the AI Blueprint generation pipeline in `quotation-api`.

---

## 1. Core Principles and Safety Invariants

1. **Tenant Isolation:** All operations (polling, retrying, manual completion, and diagnostic queries) are strictly scoped by `company_id`. Never cross tenant boundaries.
2. **Explicit User Recovery:** Provider retries and manual completions must always be explicit operator/user actions. Polling and automatic recovery cycles must never trigger AI provider calls.
3. **Attempt Budget Limit:** A maximum of three (3) provider attempts is permitted per canonical Blueprint section. Attempts are never reset automatically or via standard operational workflows.
4. **Content Preservation:** Existing validated content and manual edits must never be overwritten or invalidated during stage retries or lease reconciliation.
5. **No Direct SQL State Mutations:** SQL access in production/staging is strictly diagnostic (read-only). Direct DB mutations (`UPDATE`, `DELETE`, status overrides, attempt resets) are prohibited without an independently reviewed and approved data-repair plan.
6. **Zero Credential / Prompt Leakage:** Never log, persist, or disclose API keys, decrypted credentials, raw prompt construction buffers, or upstream HTTP response bodies.

---

## 2. Operator Decision Table

When evaluating a stalled, blocked, or failed Blueprint generation pipeline, follow this decision table:

| State / Safe Error Code | Allowed Action | Prohibited Action |
|---|---|---|
| `generating` and not stale (< 5m elapsed) | **Wait / Poll**<br>Allow the active background execution or provider request to complete. | Do NOT spam Continue or Retry endpoints. Do NOT terminate DB locks. |
| `generating` and stale (> 5m lease timeout) | **One Continue reconciliation**<br>Invoke `POST /api/v1/blueprints/:id/revisions/:revId/continue` once to trigger server-side lease reconciliation. The backend transitions stale lease to `interrupted`. Then proceed according to section state. | Do NOT loop Continue calls. Do NOT directly execute RetrySection before lease reconciliation transitions the status to `interrupted` or `failed`. |
| `failed` + retryable + attempts < 3<br>(`provider_unavailable`, `provider_rate_limited`, `provider_timeout`, `interrupted`) | **Explicit RetrySection**<br>Invoke `POST /api/v1/blueprints/:id/revisions/:revId/sections/:section/retry` to consume one attempt and re-invoke the provider. | Do NOT auto-retry without operator confirmation. Do NOT exceed attempt 3. |
| `failed` + attempts = 3 | **Manual Completion**<br>Submit validated content via `PUT /api/v1/blueprints/:id/revisions/:revId/sections/:section/manual` to unblock downstream stages. | Do NOT attempt further provider retries (will return `409 Conflict`). Do NOT reset attempt counter in DB. |
| `security_failure` | **Manual Review; No Retry**<br>Inspect prompt inputs, source quotation text, or dependency payloads for unsafe HTML/script injection or forbidden directives. Edit source quotation or manual section content. | Do NOT retry provider stage without remediating source input. |
| `cross_validation_error` | **Edit Implicated Section; No Provider Retry**<br>The generated section violates structural or relational cross-validation rules with upstream sections. Manually edit the section in the UI editor. | Do NOT re-invoke provider without modifying underlying dependent content. |
| `pipeline_blocked` | **Resolve Earliest Failed Canonical Section**<br>Inspect the canonical pipeline sequence (`journey` &rarr; `architecture` &rarr; `state_machine` &rarr; `workflow` &rarr; `wbs`). Resolve the earliest non-validated section first. | Do NOT attempt to run downstream sections while upstream dependencies are unvalidated. |

---

## 3. Safe Error Code Reference and Remediation

Every Blueprint pipeline failure produces an exact machine-readable safe code in the API envelope `errors` field. Use the corresponding remediation:

| Safe Code | HTTP Status | Description | Required Remediation |
|---|---|---|---|
| `provider_configuration` | 422 | No active AI provider credential configured for company or skill not found. | Navigate to AI Settings (`/settings/ai-provider`) and configure/enable a valid provider and model. |
| `provider_invalid_credential` | 422 | Upstream authentication failed (HTTP 401/403). | Re-enter the API key in AI Settings and verify credentials with "Test Connection". |
| `provider_unavailable` | 503 | Upstream service 500/502/503 or network transport failure. | Retry section if attempts < 3. If upstream is down, wait or switch provider. |
| `provider_rate_limited` | 429 | Upstream rate limit reached (HTTP 429). | Wait for provider rate limit reset window, then execute RetrySection. |
| `provider_timeout` | 504 | Upstream request exceeded connection/read timeout (HTTP 408/504). | Retry section or switch to faster model/provider. |
| `provider_request_rejected` | 422 | Upstream provider rejected request payload (HTTP 400/422). | Verify model capability, schema compatibility, or complete section manually. |
| `provider_payload_too_large` | 422 | Upstream rejected payload as too large (HTTP 413). | Reduce upstream context size, simplify quotation description, or complete section manually. |
| `prompt_construction_failed` | 422 | Prompt builder failed due to invalid input or missing snapshot dependencies. | Review quotation source data and skill prompt template version. |
| `invalid_schema` | 422 | Provider output failed JSON schema / domain validation. | Retry section (if attempts < 3) or manually fix content in editor. |
| `interrupted` | 409 / 422 | A generating lease expired or server process restarted during execution. | Verify server health and execute RetrySection. |
| `security_failure` | 422 | Malicious script / disallowed HTML tags detected in prompt or output. | Review quotation data for injection patterns. Complete manually if necessary. |
| `manual_completion_not_allowed` | 409 | Attempted manual completion on a validated section or section with invalid state. | Refresh status; verify section state before editing. |
| `execution_conflict` | 409 | Concurrent claim or optimistic locking conflict on revision/section. | Refresh blueprint status; retry action after state sync. |

---

## 4. Recovery for Observed Incident Blueprint (`5ddfb8c3-5ca0-4b74-992d-f5bbedf329a8`)

### 4.1 Baseline Incident State (Non-Secret)
- **Blueprint ID:** `5ddfb8c3-5ca0-4b74-992d-f5bbedf329a8`
- **Revision Status:** `partial_failed`
- **Section Statuses:**
  - `journey`: `validated` (Attempt: 3/3, manually completed or provider validated)
  - `architecture`: `failed` (Attempt: 1/3, Safe Error Code: `provider_unavailable`)
  - `state_machine`: `pending`
  - `workflow`: `pending`
  - `wbs`: `pending`
- **Tenant Provider Status:** Active encrypted credential present for tenant company; "Test Connection" succeeds.

### 4.2 Step-by-Step Recovery Procedure

1. **Verify Deployment:**
   Ensure the latest `quotation-api` (safe provider classification & diagnostics) and `quotation-app` (exact-code error presentation & decoupled lease recovery) are deployed.
2. **Access Blueprint in UI:**
   Navigate to the Blueprint Progress View for ID `5ddfb8c3-5ca0-4b74-992d-f5bbedf329a8`.
3. **Verify Journey Integrity:**
   Confirm that the `journey` section is marked `validated` and its diagram/narrative content is intact.
4. **Trigger Architecture Retry:**
   Click **"Coba Lagi"** (Retry) on the `architecture` section.
   - This sends `POST /api/v1/blueprints/5ddfb8c3-5ca0-4b74-992d-f5bbedf329a8/revisions/<revId>/sections/architecture/retry`.
   - The backend claims the section and increments attempt count to **2/3**.
   - The prompt builder constructs the Architecture prompt using the validated Journey output as dependency.
5. **Evaluate Outcome:**
   - **Case A: Generation Succeeds (`validated`)**
     The pipeline automatically or explicitly continues to subsequent stages (`state_machine` &rarr; `workflow` &rarr; `wbs`).
   - **Case B: Request Rejected or Payload Too Large (`provider_request_rejected` / `provider_payload_too_large`)**
     Do NOT immediately exhaust the 3rd attempt. Inspect model compatibility or provide manual Architecture diagram content via the Visual Editor (`PUT .../manual`) before consuming attempt 3.
   - **Case C: Upstream Transient Error (`provider_unavailable` / `provider_timeout`)**
     One attempt remains (Attempt 2 &rarr; 3). Retry section once upstream connectivity is restored.
   - **Case D: Max Attempts Reached (Attempt 3/3 Failed)**
     Use the UI editor to manually complete the Architecture section, unblocking the pipeline.

---

## 5. Strict Prohibition on Direct SQL State Repair

Operators and database administrators **must NEVER** perform direct manual data mutations on the production or staging databases.

### Forbidden SQL Operations
- `UPDATE blueprint_section_runs SET attempt = 0 ...` (NEVER reset attempt counters).
- `UPDATE blueprint_revisions SET status = 'validated' ...` (NEVER force revision statuses).
- `UPDATE blueprint_section_runs SET status = 'validated', content = ...` (NEVER bypass domain validators).
- `INSERT INTO company_ai_credentials ... SELECT ...` (NEVER copy encrypted keys across companies/tenants).
- `DELETE FROM blueprint_section_runs ...` (NEVER destroy audit trails).

### Rationale
- Direct database updates bypass tenant encryption boundaries, content cryptographic hashing, schema validation rules, version vector increments (`content_version`), and audit logs.
- Inconsistent section runs cause downstream prompt builders to crash with `prompt_construction_failed` or `invalid_schema`.

---

## 6. Permitted Diagnostic SQL Queries (Read-Only)

Operators may run the following read-only queries to inspect pipeline state without risking data corruption:

### 6.1 Inspect Revision and Section Run States
```sql
-- Replace :company_id and :blueprint_id with tenant-scoped UUIDs
SELECT 
    r.id AS revision_id,
    r.blueprint_id,
    r.company_id,
    r.status AS revision_status,
    r.content_version,
    s.section,
    s.status AS section_status,
    s.attempt,
    s.safe_error_code,
    s.started_at,
    s.completed_at,
    s.updated_at
FROM blueprint_revisions r
JOIN blueprint_section_runs s ON s.revision_id = r.id AND s.company_id = r.company_id
WHERE r.company_id = :company_id
  AND r.blueprint_id = :blueprint_id
ORDER BY s.created_at ASC;
```

### 6.2 Check Stale Lease Status
```sql
-- Find running sections that have exceeded the 5-minute lease threshold
SELECT 
    r.blueprint_id,
    r.id AS revision_id,
    r.company_id,
    s.section,
    s.status,
    s.started_at,
    NOW() - s.started_at AS elapsed_time
FROM blueprint_section_runs s
JOIN blueprint_revisions r ON r.id = s.revision_id AND r.company_id = s.company_id
WHERE s.status = 'generating'
  AND s.started_at < NOW() - INTERVAL '5 minutes';
```

### 6.3 Verify Company AI Provider Configuration Status (Non-Secret)
```sql
-- Check provider enablement without decrypting API keys
SELECT 
    company_id,
    provider_name,
    model_name,
    base_url,
    is_active,
    created_at,
    updated_at
FROM company_ai_credentials
WHERE company_id = :company_id;
```
