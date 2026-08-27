# R&D Test Fixtures

> **Status:** Frozen snapshot — NOT auto-synced with workspace originals.

## Source

These files are point-in-time copies from:

- `blueprint-skill/*` → from `docs/roadmap/rnd/blueprint-skill/`
- `quotation-skill/*` → from `docs/roadmap/rnd/quotation-skill/`

## Freeze Date

2026-08-27

## Why Copied Here

The originals live at workspace level (`docs/roadmap/rnd/`),
which is R&D documentation — not part of the `quotation-api` Go module.
Tests must be self-contained so `go test ./...` passes on any fresh clone
without relying on directory traversal outside the module root.

## Update Policy

These fixtures are frozen. If the workspace-level originals evolve and
you need tests to validate against new data, manually re-copy the
relevant files and update the freeze date above.

Do NOT set up automated sync — these are deterministic test inputs tied
to specific validator behavior at a point in time.