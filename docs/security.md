# Security review notes

## Implemented controls

- Human identity uses ThunderID OIDC Authorization Code + PKCE. API JWT validation pins RS256, requires `kid` and expiration, and validates issuer and audience. Application roles and mutable outlet/depot/vehicle mappings are resolved server-side.
- `AUTH_DISABLED` is rejected unless `ENVIRONMENT=local`; Compose explicitly uses `false`. The local ThunderID-compatible shim and its `waypoint` demo password are development-only and must not be exposed publicly.
- Service-to-service clients have distinct Compose development credentials and narrow scopes. Kubernetes credentials are referenced from Secrets; credentials are not generated into manifests. The Agent has no database dependency.
- Resource authorization and business rules are enforced in each service. Proof images must be PNG/JPEG with matching file signatures and have a 4 MiB file limit; the HTTP handler also caps multipart overhead.
- NGINX and Go add MIME-sniffing/frame protections, a Content Security Policy, referrer/permissions headers, NGINX hides its version, and request bodies/headers and server read/write/idle time are bounded.
- HTTP metrics and access logs use matched route templates instead of raw resource paths, avoiding resource-ID label cardinality.
- Kubernetes has default-deny policies plus explicit service and agent peer paths. Inspect the policies against the target CNI and actual pod labels before applying them.

## Deployment checks required

The production overlay is not a complete production platform. Before exposing a service, provision HTTPS OIDC/JWKS endpoints, per-service database roles and M2M credentials, TLS edge and DNS, durable database and proof storage, backup/restore, network policy enforcement, and a secret manager. Configure an exact browser origin if a separate-origin frontend/API deployment is introduced. Keep the local identity shim, HTTP listener, local passwords, and weak development M2M values private to the Compose network.

No production credentials or public URL are supplied in this workspace; production exposure was not performed.
