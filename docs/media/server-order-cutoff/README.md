# Server-clock cutoff notice — issue #174

The actual React store-manager order form was exercised with mocked authentication and API responses in a temporary Vite preview. Playwright/Chrome changed the browser clock without changing the host OS clock. All account, notification, and order data in the screenshots are fixtures.

## Before the fix

Source commit: `7a9ec2176a88e91e8fbd77b9f8cecc8d703ea84e`.

The cutoff endpoint fixture returned the same JSON (`16:00`, `Asia/Colombo`) and HTTP Date header corresponding to **26 October 2026, 15:00 Sri Lanka time** in both cases. Only the browser clock changed.

| Screenshot | Browser clock | Notice |
| --- | --- | --- |
| `before-browser-1500.png` | 15:00 | Order by 4:00 PM today for the next run. |
| `before-browser-1700.png` | 17:00 | Today's 4:00 PM cutoff has passed. |

`reproduction.json` records the identical responses and the differing notices. These images and observations were committed and published before the fix, and linked in GitHub issue #174.

## After the fix

| Screenshot | Service time | Browser clock | Result |
| --- | --- | --- | --- |
| `after-browser-1500.png` | 15:00, 26 October 2026 | 15:00, same day | Before cutoff |
| `after-browser-1700.png` | 15:00, 26 October 2026 | 17:00, same day | Before cutoff, unchanged |
| `after-server-past-cutoff.png` | 17:00, 26 October 2026 | 1 January 2010, America/New_York timezone | Cutoff has passed |
| `after-server-unavailable.png` | Unavailable | Same incorrect 2010 clock | Neutral policy notice; no device-time fallback |

Desktop captures use a 1440 × 1000 viewport. `verification.json` records the after observations and tested scenarios.

Additional browser checks covered an open page crossing 15:59:59 → 16:00, a backwards device-clock jump from 2036 to 2010, a non-Colombo browser timezone, changed cutoff policy, missing/invalid timestamps, unavailable requests, expiry after unanswered refreshes, and refresh on focus/online/visibility events.

## Behavior and verification scope

The endpoint returns `serverTime` as an RFC 3339 UTC timestamp, from the same service clock used for delivery-date adjustment, and marks the response `Cache-Control: no-store`. The UI advances this sample with `performance.now()`, checks the notice every second, refreshes every 30 seconds while visible, and discards samples older than 90 seconds. Resume/reconnection requests a fresh sample; failures produce neutral wording.

This fixes the cutoff notice. The backend already applies the cutoff when selecting the delivery date; the change does not introduce a blanket ban on submitting orders after 4 PM. Existing unrelated date inputs and date filters are outside this fix. Deploy the order-service update with or before the web update; an older response without `serverTime` safely produces neutral wording.

Validation: 383 web tests, production TypeScript/Vite/PWA build, and `go test ./services/order-service/...` passed. Browser screenshots use mocked responses; Go handler/service tests separately verify the real endpoint's timestamp, cache header, authorization, and existing delivery-date behavior.
