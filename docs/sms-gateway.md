# SMS through our own cellular gateway

Waypoint sends customer texts (delay and delivery notices) through the cellular gateway: an Android phone with a SIM
that the gateway server drives. Twilio is still supported and is used only when the gateway is not configured.

## How it is chosen

`integration-service` reads these settings at start-up.

| Variable | Meaning |
| --- | --- |
| `SMS_PROVIDER` | `gateway` or `twilio` forces that provider with no fallback. Empty means the gateway when `GATEWAY_URL` or `GATEWAY_API_KEY` is set, otherwise Twilio. |
| `GATEWAY_URL` | Gateway origin, for example `https://gateway.example.com`. HTTPS only (plain HTTP only for localhost). |
| `GATEWAY_API_KEY` | A `cgk_…` API key from the gateway dashboard. |
| `GATEWAY_GROUP` / `GATEWAY_DEVICE` | Optional: send from one device group, or one device. Not both. |
| `GATEWAY_QUEUE` | `true` (default) holds a text until the phone is online instead of failing. |
| `GATEWAY_WEBHOOK_SECRET` | The secret of the gateway webhook below. Without it, delivery results are not recorded. |

A half-configured provider is logged as `sms_provider_unavailable` and nothing is sent; it never silently falls back.

## Delivery results

The gateway calls `POST /api/v1/integrations/notifications/gateway` with an `X-CG-Signature: sha256=<hmac>` header.
`SMS_SENT` marks the notification sent, `SMS_FAILED` marks it failed (`GATEWAY_SMS_FAILED`). Other events are
acknowledged and ignored. Provider message ids from the gateway are stored as `cg:<request_id>`.

## One-time setup

1. In the gateway dashboard, create an API key and copy it (it is shown once).
2. Create a webhook for `SMS_SENT` and `SMS_FAILED` pointing at
   `https://<waypoint host>/api/v1/integrations/notifications/gateway`, and copy its secret.
3. Put `GATEWAY_URL`, `GATEWAY_API_KEY` and `GATEWAY_WEBHOOK_SECRET` in the VM's `.env`.
4. Make sure the phone's gateway app shows the device online.
5. Recreate the service: `sudo docker compose up -d --no-build --force-recreate integration-service`.
6. Check the log shows `sms_provider provider=gateway`.

Notes: the send call is not retried (it has no idempotency key, so a retry after a timeout could text twice). A customer
only gets an SMS if they consented; otherwise the notice stays in the app.
