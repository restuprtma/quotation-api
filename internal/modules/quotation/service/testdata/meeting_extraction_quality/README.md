# Meeting extraction quality corpus

This corpus is synthetic and anonymized. Names are invented organizational labels; it must never be replaced with customer meeting notes or copied into logs, telemetry, snapshots, or production fixtures.

It contains 17 records: two records for each of the eight commercial schemes plus one multi-service record. Coverage includes short, messy, bilingual, incomplete, and prompt-injection-shaped notes. Gold evidence uses only line numbers so the evaluation artifact never needs to emit a quote.

Run the evaluator from `quotation-api` with a prediction JSON produced by a non-production runner:

`go run ./cmd/meeting-extraction-quality -predictions /secure/path/predictions.json`

The evaluator writes aggregate-only metrics. A release passes only when the report has zero critical numeric false positives, scheme precision at least 95% (or every non-correct scheme result is `needs_review`), no snapshot regression, and an explicit positive quota margin. Live provider calls are deliberately outside this command.
