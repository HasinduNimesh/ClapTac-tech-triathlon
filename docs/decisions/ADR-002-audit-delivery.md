# ADR-002: Audit delivery semantics

## Status

Accepted for Milestone 2

## Decision

Order persistence is the source of truth for create success. If Shared Service audit ingest fails after the order is committed:

1. The client still receives `201` with the order.
2. The failure is logged (`correlation_id`, `order_id`).
3. Order Service retries ingest a few times in-process.

A transactional outbox is the intended later hardening. It is not required in Milestone 2.
