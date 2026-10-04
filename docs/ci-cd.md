# CI and Azure VM deployment

`.github/workflows/ci-cd.yml` checks pull requests and pushes to `main`. It
selects jobs by changed paths: Go vet/tests/build for backend changes, web
tests/build for web changes, Flutter analysis/tests for mobile changes, and
Compose config plus affected image builds for deployable changes. Workflow and
Makefile changes run every check. Documentation-only changes do not rebuild
applications. These jobs do not receive production secrets. A manual deployment
runs the complete check set regardless of changed paths. Production deployment
runs only when **Actions → CI and
production deployment → Run workflow** is started on `main` and all checks pass.

## Images

Every push to `main` that passes its checks builds all ten images (the eight
Go services, the web app and the Flutter loader) on GitHub and pushes them to
`ghcr.io/<owner>/<repo>/<service>`, tagged with the full commit SHA. There is no
`latest` tag: a tag always names one commit. The VM never builds; it pulls the
images for the commit being deployed.

The web app and the loader bake their sign-in addresses and the API audience in
at build time, so those two images are for production. The defaults are
`https://id.waypoint.claptac.dev`, client `waypoint-web`
(`waypoint-loader` for the loader), `https://waypoint.claptac.dev/auth/callback`
and the audience `https://waypoint.claptac.dev/api/v1`. The audience must equal
`OIDC_AUDIENCE` in the VM `.env`: the deploy refuses to start if it does not. If
the production values differ, set the repository variables `PROD_OIDC_ISSUER`,
`PROD_OIDC_AUDIENCE`, `PROD_WEB_OIDC_CLIENT_ID`, `PROD_LOADER_OIDC_CLIENT_ID` and
`PROD_WEB_REDIRECT_URI` (Settings → Secrets and variables → Actions →
Variables). They are public addresses, not secrets.
Local `docker compose up --build` still builds development images tagged
`waypoint/<service>:local`.

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

## Let the VM pull images (once)

Packages pushed by Actions are private. Create a **classic** personal access
token with only `read:packages` (fine-grained tokens cannot read packages), then
on the VM, as the account the deploy uses `sudo docker` for:

```
echo '<token>' | sudo docker login ghcr.io -u <github-user> --password-stdin
```

The credential stays in root's Docker config on the VM; it is not stored in
GitHub. The token's owner needs read access to the repository.

## Confirm what the VM actually runs (before anything else)

The presence of `.env.production` or `docker-compose.production.yml` on the VM
does not mean they are in use. This workflow assumes the *active* setup is plain
`docker-compose.yml` with `.env`. Check it from the running containers (read-only):

```
sudo docker inspect $(sudo docker compose ps -q nginx) \
  --format '{{index .Config.Labels "com.docker.compose.project.config_files"}} | {{index .Config.Labels "com.docker.compose.project.environment_file"}}'
sudo docker ps --format '{{.Names}}\t{{.Image}}'
```

If this shows an overlay file or a different env file, or storage other than what
`.env` configures, stop: the deploy job has to be changed to match. Moving the VM
to the production overlay and R2 (PR #18) is a separate infrastructure decision.

## One-time checkout move

The deploy fast-forwards the VM checkout to `main` and **refuses to run if the
checkout is on another branch**. The VM is on `codex/public-deployment` (PR #18,
not merged) with a local edit to `docker-compose.yml`. Move it deliberately, with
a way back:

1. Keep what is there: `git branch backup/vm-$(date +%Y%m%d)` and
   `git diff > ~/vm-local-changes.patch` (and `git stash list` if anything is stashed).
2. The only thing the local Compose edit needs to preserve is the nginx port. Set
   `NGINX_BIND=127.0.0.1:8080:80` in `.env`; `main`'s Compose reads it.
3. Check which files exist only on that branch (`git diff --stat main...HEAD`),
   for example `scripts/production/create_user.py`, the Caddy file and the
   production docs. They will disappear from the working tree when you switch.
   Copy any you still use out of the repository first.
4. `git checkout -- docker-compose.yml && git fetch origin && git checkout main && git merge --ff-only origin/main`.
5. `sudo docker compose config --quiet`, and confirm the effective ports and the
   OIDC and audience values before running the workflow.

This has not been tried on the VM; do it before the first run and tell the
workflow's author what differed.

## VM assumptions and rollout

The job expects `~/ClapTac-tech-triathlon`, an existing `.env`, Docker Compose,
Caddy, and `NGINX_BIND=127.0.0.1:8080:80` in `.env`. It fetches `main` and
fast-forwards the VM checkout to the *exact* commit checked by CI. A divergent
branch or Compose conflict stops deployment. It validates Compose, pulls the
images for that commit (stopping before anything changes if one is missing or
the registry login is not set up), checks that the web and loader images carry
the production sign-in address and API audience, saves a PostgreSQL custom-format
dump in the VM user's home directory, **runs the migrations** (`migrate`) before
any updated service starts, writes `WAYPOINT_IMAGE_PREFIX` and `WAYPOINT_TAG`
into the VM `.env` (only those two lines), and starts Compose without building.
It then requires every service and nginx to report healthy, and checks
`/health/live`, `/loader-app/` and the identity server's discovery document.
Compose also has to show `OIDC_AUDIENCE` equal to the audience above. It does not remove
orphan containers: `waypoint-thunderid-prod` is managed separately.

The VM's current local Compose edit will block this update to that file. Before
the first automated deployment, set `NGINX_BIND=127.0.0.1:8080:80` in the VM's
`.env`, then reconcile the local Compose edit with `main` while preserving the
running port mapping. Review and preserve the VM's
existing `.env` and database; never reset the repository or delete volumes to
make a deployment pass. A successful health check is not a full user-flow
check, and this workflow does not perform an automatic rollback.

To roll back, set `WAYPOINT_TAG` in the VM `.env` to the previous commit SHA
(a failed deploy prints it; earlier tags are still in the registry) and run
`sudo docker compose up -d --no-build`. Migrations only go forward and the backup
is the way back for data, kept separate from image rollback: check that the older
images work with the current schema before rolling back across a migration.

The deploy preflight requires the effective web issuer and redirect URI, plus
the API issuer, JWKS URL, and token endpoint, to point to the production
ThunderID host. The Compose file now accepts those values from `.env` instead
of forcing development URLs. Existing local defaults remain for development.
The preflight refuses deployment before taking a backup or restarting services
if the production values are missing.

The Flutter app is checked by CI but not deployed by Docker. Shipping a driver
release still requires a signed Android build and an app distribution process.
