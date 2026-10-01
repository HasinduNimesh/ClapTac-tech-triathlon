# kGateway

Kubernetes API gateway (Gateway API). Not used in local Compose.

Responsibilities:

- API routing by domain prefix (`/api/v1/orders`, `/planning`, `/fleet`, `/loading`, `/delivery`, `/shared`, `/integrations`, `/agent`)
- JWT / API policies
- Service routing
- API rate policies
- Telemetry

NGINX remains the external edge (TLS, static web, basic limits).

Gateway and HTTPRoute manifests are maintained in
`infrastructure/kubernetes/base/gateway/` so the Kustomize base can build with
its default safe loader.
