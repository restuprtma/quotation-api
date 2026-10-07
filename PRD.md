# Backend Product Contract - Quotation & Blueprint Generator (AI-First)

**Audience:** Backend Engineers, System Architects, Quality Engineers.  
**Version:** 2.0 (Phase 13 Canonical)  
**Status:** Active Canonical Contract  

---

## 1. Core Architecture & Responsibilities

The Quotation & Blueprint Generator backend is a multi-tenant modular platform built with Go 1.25, Gin, PostgreSQL (`pgxpool`), and Redis.

1. **AI-First Skill Architecture:**
   - `/quotation` SKILL produces clean, sanitized HTML fragments. HTML is the canonical content stored in `quotation_revisions(content_html)`.
   - `/blueprint` SKILL produces a structured Blueprint Bundle JSON. JSON is the canonical content stored in `blueprint_revisions(content_json)`.
2. **Rate-Card Authority:**
   - AI NEVER determines prices, rates, discounts, or totals.
   - Rates are resolved from official company rate cards (`rate_cards`, `rate_card_items`) based on duration:
     - Durasi < 3 bulan: **Short Term** rates.
     - Durasi ≥ 3 bulan: **Long Term** rates.
   - Client budget is an internal maximum limit; it is NEVER displayed on the quotation document.
   - The breakdown displayed to the client is **Total Investment**, where `Total Investment <= budget_maximum`.
3. **Legal Template Authority:**
   - Legal clauses originate from official versioned templates (`legal_templates`). AI is strictly forbidden from paraphrasing or altering legal terms.
   - Legal blocks are read-only by default; BA manual unlock is audited via `legal_content_modified`.
4. **Lifecycle & Immutability:**
   - Draft revisions are editable via TipTap / Visual Editors (`Save Draft` does not increment revisions).
   - Publishing locks the revision into an immutable state (`published`).
   - Subsequent changes require creating a new draft revision (`revisions`).

---

## 2. Public API & Sharing Contract

The public surface exposes strictly allowlisted, unauthenticated endpoints guarded by cryptographically secure tokens:

```text
GET  /p/q/:public_token
POST /p/q/:public_token/sign
GET  /p/blueprint/:token
```

### 2.1 Public Quotation (`/p/q/:public_token`)
- **Stable Link:** Resolves atomically to the active published quotation revision.
- **DTO Allowlist:** `quotation_number`, `subject`, `client.company_name`, `revision_number`, `content_html`, `published_at`, `signed`, `signer_name`, `signed_at`.
- **Zero Leakage:** Excludes `company_id`, internal IDs, `budget_maximum`, generation snapshots, AI prompts, and audit IPs.
- **Digital Signature (`/sign`):** Atomically locks rows (`FOR UPDATE`), validates signer info and PNG data, records SHA-256 `signed_content_hash`, and rejects double-sign attempts with `409 Conflict`.

### 2.2 Secure Blueprint Sharing (`/p/blueprint/:token`)
- **Token CSPRNG:** Token is generated with ≥32 bytes of CSPRNG entropy (`crypto/rand`).
- **Database Storage:** Database stores ONLY the lowercase hex SHA-256 hash in `blueprint_share_links(token_hash)`. Raw tokens are NEVER stored in DB or logged.
- **Revision Binding:** Link is permanently bound to a specific `blueprint_revision_id` and does not float to newer revisions.
- **Programmer DTO Allowlist:** Excludes quotation numbers, client names, budgets, rates, internal IDs, AI prompts, and revision history.

---

## 3. Purged Legacy Contracts (Historical / Superseded)

The following legacy concepts are completely superseded and must NOT be used:
- **Legacy Relational Pricing:** Relational pricing items/sections (`pricing_items`, `line_total = unit_price * quantity`) are purged.
- **Legacy Three Artifacts:** `/p/b/:token` and `/p/d/:token` are removed.
- **Legacy Generation Route:** `/quotation/v1/ai/generate` and `POST /quotation/v1/quotations/:id/share` are disabled.
