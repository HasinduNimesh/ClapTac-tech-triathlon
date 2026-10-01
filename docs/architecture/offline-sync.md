# Offline synchronization

The driver web PWA is offline-first for Milestone 5. The Flutter app remains a later client; the AI agent is not on this path.

Operational data lives in IndexedDB (Dexie). The service worker caches the app shell only (`js`, `css`, `html`, icons) and never caches `/api`.

FIFO queue:

1. `ARRIVED`
2. multipart `PROOF_UPLOAD` (never as `/sync` JSON)
3. `STOP_OUTCOME` with `dependsOnOperationId` pointing at the proof
4. `ROUTE_COMPLETED`

Every operation has a client-generated `operationId`. Replaying the same id returns `DUPLICATE` with `originalStatus` and must not overwrite `APPLIED`. A 401 pauses the queue without wiping IndexedDB.

The delivery-service `POST /api/v1/delivery/sync` endpoint is the JSON contract. Proof bytes use `POST /api/v1/delivery/trips/{tripId}/stops/{stopId}/proofs`.
