# Waypoint production VM

The public app is `https://waypoint.claptac.dev`; the OIDC issuer is
`https://id.waypoint.claptac.dev`. Both DNS A records point to the VM. The Azure
NSG permits inbound TCP 80 and 443. Keep SSH available for administration and
restrict its source IP when possible. Do not expose 8080, 8091, 5432, or 6379.

## Components

- Caddy terminates HTTPS and proxies the app to loopback port 8080 and the
  public ThunderID routes to loopback port 8091. Install Caddy from its official
  package repository and place `infrastructure/caddy/Caddyfile.production` at
  `/etc/caddy/Caddyfile`, then validate and reload it.
- `docker-compose.yml` plus `docker-compose.production.yml` run the app, its
  existing PostgreSQL and Redis volumes, and real ThunderID. The demo seed,
  bootstrap, ThunderID shim, and S3 mock use the `local-only` profile.
- ThunderID's private config, signing material, and resource definitions live
  under `${THUNDERID_PRIVATE_DIR}` on the VM. The production Compose file
  expects its `container.env`, `deployment.yaml`, `certs`, `secrets`, and
  `resources` directories there. Preserve these when backing up or replacing
  the VM. Its four PostgreSQL databases are separate from Waypoint's database.
- Delivery proof objects use a private Cloudflare R2 bucket. Its bucket-scoped
  credentials are in the VM's owner-readable `.env.production`, never in Git.

## Deploy an update

1. Back up the existing Waypoint and four ThunderID databases, plus the private
   ThunderID config and proof objects. Keep a copy off the VM.
2. Update the checkout. Keep `.env.production` mode 600. Set the values in
   `.env.production.example`, including `THUNDERID_PRIVATE_DIR`, to the real
   deployment values. `VITE_*` values are compiled into the web image.
3. From the repository root, run:

   ```sh
   sudo docker compose --env-file .env.production \
     -f docker-compose.yml -f docker-compose.production.yml config --quiet
   sudo docker compose --env-file .env.production \
     -f docker-compose.yml -f docker-compose.production.yml up -d --build
   sudo docker compose --env-file .env.production \
     -f docker-compose.yml -f docker-compose.production.yml ps
   ```

4. Check `https://waypoint.claptac.dev/health/live`,
   `https://id.waypoint.claptac.dev/.well-known/openid-configuration`, and a
   browser sign-in. The identity console is intentionally unavailable through
   the public Caddy proxy.

Do not run `docker compose down -v`, the demo seed, or the demo bootstrap against
production. The application database and ThunderID databases hold live data.

## Initial account and current limits

An operator account is provisioned in ThunderID and mapped to a Dispatcher
profile in Waypoint. Its username and protected password are stored on the VM;
retrieve the password over SSH, then change it through the identity system once
sign-in is confirmed. Public self-registration and email-based recovery are
disabled until SMTP, registration policy, and account lifecycle are configured.
Automated off-VM backups and monitoring still need to be set up.
