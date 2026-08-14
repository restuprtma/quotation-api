# Backend AI Contract

The AI provider interface returns a validated bundle with `schema_version`, `narrative`, `product_or_project_detail`, `blueprint`, and `pricing_labels`.

The provider never owns pricing. The service passes manual pricing as read-only context and preserves it. The validator limits field lengths/counts, permits only supported diagram types (`journey`, `architecture`, `state_machine`, `workflow`, `timeline`, `comparison`), and rejects unsafe HTML/script markers.

Provider modes:

- `mock`: deterministic CI/development.
- `ollama`: local/private, no API token cost.
- `gemini`: hosted free-tier prototype after current quota review.
- `openrouter`/`groq`: optional hosted providers.

Provider keys are backend-only environment variables.
