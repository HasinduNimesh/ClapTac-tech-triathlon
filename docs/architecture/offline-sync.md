# Offline synchronization

The driver web PWA is offline-first for Milestone 5. The Flutter app remains a later client; the AI agent is not on this path.

Operational data lives in IndexedDB (Dexie). The service worker caches the app shell only (`js`, `css`, `html`, icons) and never caches `/api`.

Send order (`apps/web/src/offline/syncQueue.mjs`, `orderForSync`): saved records go first, then photos. In practice:

1. records: `START`, `ARRIVED`, temperature, incidents, Tech custody at dispatch, and failed/refused outcomes, in the order the driver made them
2. multipart `PROOF_UPLOAD` (never as `/sync` JSON)
3. `STOP_OUTCOME` with `dependsOnOperationId` pointing at the proof, and Tech custody that cites a photo. The delivery service rejects a delivered/partial outcome until its proof has arrived, so these wait behind their own photo
4. `ROUTE_COMPLETED`, always last

Nothing leaves the queue until the server accepts it. A failed upload keeps its record, the stop shows "Could not send" with a Retry button, and Retry resumes the queue in the same order.

Each stop shows "Saved on this phone", "Sending" or "Sent", and the page shows how many items are waiting. Every queued record is stamped with `planVersion`, the plan version the driver screen was showing when it was made (current plan version, else the run's). `/sync` operations send it; when the server reports `conflict{recordedPlanVersion,currentPlanVersion,detail}` the driver keeps the record, and a notice ("Recorded on plan v1 - the current plan is v2. Your record was kept; dispatch will review it.") is stored in IndexedDB `meta` and shown beside that stop until the driver dismisses it. The dispatcher settles the conflict from the Notifications page.

Every operation has a client-generated `operationId`. Replaying the same id returns `DUPLICATE` with `originalStatus` and must not overwrite `APPLIED`. A 401 pauses the queue without wiping IndexedDB.

The delivery-service `POST /api/v1/delivery/sync` endpoint is the JSON contract. Proof bytes use `POST /api/v1/delivery/trips/{tripId}/stops/{stopId}/proofs`.
