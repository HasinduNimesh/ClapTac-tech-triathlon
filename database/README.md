# Database

Waypoint uses **one PostgreSQL instance** in Milestone 1.

Logical schema ownership:

| Schema | Owner service |
|---|---|
| `orders` | order-service |
| `planning` | planning-service |
| `fleet` | fleet-service |
| `loading` | loading-service |
| `delivery` | delivery-service |
| `shared` | shared-service |
| `audit` | shared-service |

Services must not query another schema. Example: delivery-service must not `SELECT` from `orders.orders`; it calls Order Service over HTTP.

`0001_init.sql` creates extensions and schemas only. Domain tables arrive with Milestone 2+.

Do not create a database per service yet.
