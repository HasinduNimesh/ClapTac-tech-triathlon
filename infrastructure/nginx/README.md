# NGINX

Local development and Kubernetes external edge.

Responsibilities:

- TLS termination (when certificates are provided)
- Static / SPA hosting via the web upstream
- Edge headers
- Basic request limits

NGINX does **not** replace kGateway. In Architecture A / Kubernetes:

```
Internet → NGINX / external edge → kGateway → services
```

Local Compose:

```
Browser → NGINX → services
```
