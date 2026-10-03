# Data model and seed data

Waypoint uses one PostgreSQL instance with schema ownership documented in [database/README.md](../database/README.md): `orders` (Order Service), `planning` (Planning Service), `fleet` (Fleet Service), `loading` (Loading Service), `delivery` (Delivery Service), and `shared` plus `audit` (Shared Service), and the new `inventory` and `ai` schemas for store stock and agent data (tables only until their services exist). References across owned schemas are opaque identifiers; services use HTTP APIs instead of cross-schema queries. The Agent has no database connection.

Migrations are additive in `database/migrations/`; `database/apply.sh` records each filename and skips it on subsequent starts. Compose applies migrations, fixture seeds, and ThunderID subject/profile bootstrap in dependency order.

The files in `database/seeds/` are competition-shaped development fixtures, not identified as the official competition dataset. Current fixture counts: 120 outlet rows, 60 vehicles, 1,096 calendar dates, 100 district-travel pairs, and 2 service-allowance rows. `./scripts/validate-seeds.py` checks unique IDs/dates, required demo outlets, depot references, numeric bounds, and calendar value shapes before import. No official dataset is present in this checkout; the replacement path is `make seed-competition-data`, after replacing the documented CSVs with the official files.

The seeded delivery day is created during the walkthrough; the demo accounts and reference fleet are seeded, while orders/plans/routes are generated through the product workflow. Vehicle availability is operational data separate from static vehicle fixture rows.

The product catalog, order and receipt lines, store stock ledger, simulated sales history and the agent tables (`ai.observations`, `ai.insights`, `ai.suggestions`) are described in [database/README.md](../database/README.md#catalog-store-stock-and-agent-data-migrations-00500053), including units, ownership and how the seed data is generated.
