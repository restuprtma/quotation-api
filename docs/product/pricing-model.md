# Backend Pricing Rules

Allowed modes:

- `unit`: `unit_price * quantity`.
- `resource`: `unit_price * employee_count * duration`.
- `fixed`: `unit_price`.

Allowed billing periods: `one_time`, `monthly`, `yearly`, `custom`.

Server recalculates totals using integer Rupiah values and rejects negative values or integer overflow. AI output cannot override any pricing input. Return one-time and recurring totals separately.
