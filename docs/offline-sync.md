# Offline sync

The required driver client is the React PWA; a Flutter shell is optional and remains unverified when Flutter is unavailable. Operational route data and a FIFO operation queue live in IndexedDB. The service worker caches only the application shell and does not cache `/api` responses.

While disconnected, the driver can record arrival, outcome, proof metadata/upload work, and route completion as operations with stable operation IDs. Reconnection replays the queue in dependency order. The server makes sync operations idempotent, reports conflicts explicitly, and the UI keeps queued work when a 401 requires sign-in. Proof bytes use the dedicated multipart endpoint rather than JSON sync.

For the strongest browser check, sign in as `driver / waypoint`, open Driver → Trips, use browser DevTools to set network to Offline, record arrival and proof/outcome work, then reconnect and select **Sync Now**. Verify the queue clears only after acknowledgement and the end-of-day summary reflects the outcome. Do not clear browser storage during the test.

If the browser cannot emulate offline mode, a local delivery-service outage provides a useful recovery check without clearing IndexedDB: start the route while online, stop only the Compose `delivery-service`, record arrival and proof/outcome work, restart that service, select **Sync Now**, and verify the server-backed trip summary. This tests API-outage persistence and recovery; it does not emulate loss of all browser network connectivity. The M8 browser run recorded in [submission evidence](submission.md) used this outage method.

See the detailed [sync contract](architecture/offline-sync.md).
