# CI and Azure VM deployment

`.github/workflows/ci-cd.yml` checks every pull request and every push to `main`:
Go vet, tests, and build; web tests and build; Flutter analysis and tests; and
Docker Compose configuration and image builds. These jobs do not receive
production secrets. A production deployment runs only when **Actions → CI and
production deployment → Run workflow** is started on `main` and all checks pass.

## Configure production access

Create a GitHub environment named `production` and require a reviewer. Add
these **environment secrets**:

| Secret | Value |
| --- | --- |
| `AZURE_SSH_HOST` | VM public IP or SSH hostname |
| `AZURE_SSH_USER` | `azureuser` |
| `AZURE_SSH_PRIVATE_KEY` | A dedicated deployment private key; add its public key to the VM user's `~/.ssh/authorized_keys` |
| `AZURE_SSH_KNOWN_HOSTS` | A verified `known_hosts` entry for the VM, including its host key |

Do not reuse the personal VM key or paste application `.env` into GitHub. The
VM retains its own `.env`; the deploy job does not print or copy it. Grant the
deployment account passwordless `sudo docker` access only if the account is
trusted to control this VM. Restrict SSH access to the runner's network or use
a self-hosted runner/VPN if the VM firewall does not permit GitHub runners.
Verify the VM host key through an independent trusted channel before saving
`AZURE_SSH_KNOWN_HOSTS`; do not accept `ssh-keyscan` output blindly.

## VM assumptions and rollout

The job expects `~/ClapTac-tech-triathlon`, an existing `.env`, Docker Compose,
Caddy, and `NGINX_BIND=127.0.0.1:8080:80` in `.env`. It fetches `main` and
fast-forwards the VM checkout to the *exact* commit checked by CI. A divergent
branch or Compose conflict stops deployment. It validates Compose, saves a
PostgreSQL custom-format dump in the VM user's home directory, rebuilds and
starts Compose, then checks the public health endpoint. It does not remove
orphan containers: `waypoint-thunderid-prod` is managed separately.

The VM's current local Compose edit will block this update to that file. Before
the first automated deployment, set `NGINX_BIND=127.0.0.1:8080:80` in the VM's
`.env`, then reconcile the local Compose edit with `main` while preserving the
running port mapping. Review and preserve the VM's
existing `.env` and database; never reset the repository or delete volumes to
make a deployment pass. A successful health check is not a full user-flow
check, and this workflow does not perform an automatic rollback.

The deploy preflight requires the effective web issuer and redirect URI, plus
the API issuer, JWKS URL, and token endpoint, to point to the production
ThunderID host. The Compose file now accepts those values from `.env` instead
of forcing development URLs. Existing local defaults remain for development.
The preflight refuses deployment before taking a backup or restarting services
if the production values are missing.

The Flutter app is checked by CI but not deployed by Docker. Shipping a driver
release still requires a signed Android build and an app distribution process.
