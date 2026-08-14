# Backend Security Rules

- Internal routes use JWT and `CompanyContext()`.
- Public routes do not use JWT/company context but use UUID artifact tokens.
- All internal queries filter by `company_id`.
- Public DTOs never expose tenant ID, creator ID, raw AI response, internal notes, or signed IP.
- Signature data is bounded, PNG data-URL validated, and stored with server timestamp/IP.
- Signed records cannot be overwritten.
- AI provider credentials stay in backend environment variables.
