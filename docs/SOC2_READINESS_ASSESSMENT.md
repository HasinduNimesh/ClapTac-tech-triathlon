# Waypoint — SOC 2 readiness assessment

**Type:** engineering gap analysis from the repository and GitHub settings. It is not an audit, an attestation or legal advice.
**Basis:** the `bug-fixes` branch, treated as `main`. Evidence is the code, config, CI workflow and `gh api` settings checks. I did not inspect the live Azure VM, the production ThunderID tenant, or any policy documents held outside the repo.

## 1. Verdict

**Not SOC 2 ready.** No SOC 2, ISO 27001 or PCI effort exists in the repo. The *product* has a strong set of technical controls for a hackathon build. The gaps that matter most are the missing *organizational* controls and *evidence*, plus a few technical defaults that auditors would flag.

SOC 2 attests to how an organization operates over a period of time (Type II is typically 3–12 months). Code alone can't satisfy it.

| Trust Services Criteria | Technical readiness | Organizational readiness | Overall |
|---|---|---|---|
| Security (Common Criteria, mandatory) | Partial | Not started | **Red** |
| Availability | Partial | Not started | **Red** |
| Confidentiality | Partial | Not started | **Amber/Red** |
| Processing integrity | Good | Not started | **Amber** |
| Privacy | Partial | Not started | **Red** |

Recommended scope for a first report: **Security + Availability + Confidentiality**, Type I first. Add Processing Integrity and Privacy later if customers ask for them.

## 2. Where Waypoint is already strong

| Control | Evidence | Maps to |
|---|---|---|
| Strong authentication protocol | OIDC authorization code + PKCE, RS256 only, `kid`/`exp`/issuer/audience/`sub` required (`pkg/auth`) | CC6.1 |
| Layered authorization | Authenticate → RBAC → resource scope → business rules in every service; role read from the DB, not the token | CC6.1, CC6.3 |
| Least-privilege service identity | Per-service OAuth client and narrow scopes; the AI tier forwards the user's token and holds no secrets or DB | CC6.1, CC6.3 |
| Fail-closed switches | `AUTH_DISABLED` refused unless `ENVIRONMENT=local`; durable object storage required outside local | CC6.1, CC8.1 |
| Tamper-evident audit | Append-only `audit.events` with SQL tests (`database/tests/append_only.sql`); `audit:read` permission; audit catalogue | CC7.2, CC4.1 |
| Request integrity | Idempotency keys, content-hashed plan versions, strict JSON, size limits, PNG/JPEG signature checks | PI1.2–PI1.5 |
| Edge hardening | NGINX CSP, nosniff, frame-deny, referrer and permissions policy, 50 req/s rate limit, server version hidden | CC6.6, CC6.7 |
| Webhook authenticity | Twilio signature and gateway HMAC; refuse when unconfigured | CC6.6 |
| Client token storage | Android Keystore; `sessionStorage` on web; session and inactivity timeouts (loader 15 min, web 1 hour with warning) | CC6.1 |
| Monitoring | Prometheus alerts, Grafana, OTel traces, route-template metric labels (no PII in labels) | CC7.1, CC7.2, A1.1 |
| Change gate | CI runs `go vet`, `go test`, build, web tests, Flutter analyze and test; images tagged by full commit SHA; deploy preflight rejects dev-IdP settings, requires matching commit, uses a GitHub `production` environment and pinned SSH known-hosts | CC8.1 |
| Backup tooling | Postgres and object-storage backup scripts, with restore-verification scripts and tests | A1.2, A1.3 |
| Data minimization | Live location keeps the latest point only; SMS phone and body erased 7 days after the terminal attempt; proof-retention worker | C1.2, P4, P6 |

## 3. Gaps, ranked

### Critical (would likely produce audit exceptions or real risk)

| # | Gap | Evidence | Criteria | Fix |
|---|---|---|---|---|
| C1 | **No branch protection** on `bug-fixes` or `main`; no required reviews or required checks | `gh api …/branches/bug-fixes/protection` → 404 "Branch not protected" | CC8.1 | Require PR, ≥1 approval, passing CI, no force-push, and signed commits if possible; apply to the real release branch |
| C2 | **Production deploys and image publishes are allowed from two branches** and are manually triggered by anyone with the workflow permission | `ci-cd.yml`: `main` or `bug-fixes`; deploy needs only `workflow_dispatch` + `production` environment | CC8.1 | One release branch; add required reviewers on the `production` environment |
| C3 | **No vulnerability or secret scanning anywhere** | CI has no `govulncheck`, `npm audit`, Trivy, CodeQL or gitleaks. GitHub: secret scanning, push protection and Dependabot security updates all `disabled`. No `dependabot.yml` | CC7.1, CC3.2 | Enable GitHub secret scanning and push protection, Dependabot (gomod, npm, pub, pip, docker, actions), `govulncheck` and an image scan in CI |
| C4 | **Insecure defaults for credentials and transport** | `docker-compose.yml` defaults: DB user/password `waypoint/waypoint`, `sslmode=disable`, M2M secrets `local-*-m2m`, Grafana `admin/admin`, OpenSearch `WaypointDev_1`. The preflight guards the IdP URL but does not reject default secrets | CC6.1, CC6.7 | Make production refuse to start with default secrets; require `sslmode=verify-full`; use a secret manager; rotate |
| C5 | **Single shared database role** serves all nine schemas (fix built but not yet adopted, see section 8) | Security notes; no `CREATE ROLE`/`GRANT` in migrations | CC6.1, CC6.3 | Per-service DB roles limited to their own schema; read-only role for support; `audit` insert-only for services |
| C6 | **No documented policy set or evidence trail** | No security policy, access review, incident response, risk assessment, vendor, HR or BCP/DR documents in the repo. `docs/security.md` is a technical note | CC1–CC5, CC9 | See section 5 |

### High

| # | Gap | Evidence | Criteria | Fix |
|---|---|---|---|---|
| H1 | **No MFA in the product**; depends entirely on the production IdP (ThunderID) config, which is unverified | No MFA/TOTP references in code; local dev IdP uses a shared demo password | CC6.1 | Enforce MFA in the IdP for dispatcher and admin roles; document it |
| H2 | **Encryption at rest unspecified** for Postgres, object storage, backups and the driver's SQLite queue | Driver README: queue "not separately encrypted"; no storage-encryption config | CC6.1, C1.1 | Encrypted volumes or managed DB encryption; encrypt backups; SQLCipher or encrypted file store on the phone |
| H3 | **Single-VM deployment**; no HA, no tested DR objective | Azure VM + Compose is the live path; Kubernetes is only a target; no RTO/RPO; Redis unused | A1.2, A1.3 | Define RTO/RPO, run and record a restore test, monitor uptime, plan a second node or managed DB |
| H4 | **No session/log retention and access-log policy**; audit archival undefined | Privacy inventory "retention not defined" for orders, delivery events and audit | CC7.2, C1.2, P4 | Set retention per data class; ship logs to central, access-controlled storage |
| H5 | **Audit schema/contract drift**: `actor_type` enum lacks `human` and `workflow` values the code writes | `contracts/events/audit-event.schema.json` vs code | CC7.2, PI1 | Align enum and schema; add a contract test |
| H6 | **Public repository** with test credentials in `README.md` and a rich architecture description | `gh api` → `visibility: public` | CC6.1, C1.1 | Rotate or remove any credential that touches a real environment; decide whether the repo should stay public |
| H7 | **Agent approval state is in process memory** and lost on restart | Security notes | PI1, CC7.2 | Persist approvals and their audit records |

### Medium

| # | Gap | Fix |
|---|---|---|
| M1 | Privacy inventory is stale (still says no continuous location) and lacks a lawful basis, data-subject process, cross-border analysis and device-unbinding process | Update and have the data owner and a qualified privacy reviewer sign it |
| M2 | Third-party/sub-processor inventory missing (Twilio, OpenAI-compatible LLM, OpenStreetMap tiles, GHCR, Azure, ThunderID) with no vendor reviews or DPAs | Build a vendor register; collect their SOC 2 reports |
| M3 | Proof-retention worker is off by default (`DELIVERY_PROOF_RETENTION_ENABLED=false`) | Enable per the chosen retention period |
| M4 | AI assistants send order and dashboard context to an external model | Document data sent, provider terms and the opt-out; keep the allow-listed tool set |
| M5 | Operational alerts for the Android offline queue are dormant (the app doesn't emit the telemetry) | Emit the telemetry or remove the alerts |
| M6 | Compose publishes only Postgres on localhost but network policy is only enforced in the Kubernetes target | Enforce network segmentation on the real host (firewall/security groups) |
| M7 | No penetration test or threat model | Commission one before the audit window |
| M8 | Test and tooling artefacts (screenshots, `.mcp.json`, `.codex/`) sit untracked in working copies | Keep them out of the repo and the release build |

## 4. Criteria-by-criteria summary

| Criterion | Status | Notes |
|---|---|---|
| CC1 Control environment | Not started | No code of conduct, org chart, roles/responsibilities or board oversight evidence |
| CC2 Communication | Not started | No internal/external security communication process |
| CC3 Risk assessment | Not started | No risk register; a threat model is not documented |
| CC4 Monitoring | Partial | Metrics and alerts exist; no control-monitoring or review cadence |
| CC5 Control activities | Partial | Technical controls exist; no written policies |
| CC6 Logical and physical access | Partial | Good authN/authZ design; shared DB role, default secrets, no MFA evidence, no access reviews, no offboarding process. Physical access falls to Azure (inherit their report) |
| CC7 System operations | Partial | Monitoring and audit trail; no vulnerability management, no incident response plan |
| CC8 Change management | Partial | CI and SHA-tagged images; no branch protection or review enforcement |
| CC9 Risk mitigation | Not started | No vendor management or business-disruption insurance/plan |
| A1 Availability | Partial | Backups and alerts; single VM, no RTO/RPO, no DR test |
| C1 Confidentiality | Partial | Scoping and secrets hygiene are decent; no encryption-at-rest evidence, no retention |
| PI1 Processing integrity | Good | Idempotency, validation, ledgers, deterministic planner; fix the audit schema drift |
| P1–P8 Privacy | Partial | Minimization and SMS erasure exist; no notice, consent records, subject-rights or retention decisions |

## 5. Organizational controls to create (no code needed)

An auditor will ask for these documents and for evidence they are followed:

1. Information security policy and acceptable-use policy
2. Access control policy: joiner/mover/leaver process, **quarterly access reviews** (GitHub, Azure, ThunderID, production DB)
3. Risk assessment methodology and risk register
4. Incident response plan with a tested tabletop exercise
5. Change management policy (maps to the CI/CD and PR rules)
6. Vendor management policy and register
7. Business continuity and disaster recovery plan, with a recorded restore test
8. Data classification, retention and disposal policy
9. Secure development policy (code review, dependency and secret scanning)
10. Security awareness training and background-check records for staff
11. Encryption and key-management standard
12. Logging and monitoring standard, including who reviews alerts

## 6. Suggested roadmap

| Phase | Time | Work |
|---|---|---|
| 0. Decide | Week 1 | Scope (Security + Availability + Confidentiality), Type I vs II, auditor, a compliance tool (Vanta, Drata or similar), single release branch |
| 1. Quick technical wins | Weeks 1–3 | C1, C2, C3, H5, H6, M3; delete stray artefacts; enable GitHub secret scanning and Dependabot |
| 2. Harden production | Weeks 3–8 | C4, C5, H1, H2, H4, M6; secret manager; per-service DB roles; TLS to DB; encryption at rest; central logs |
| 3. Resilience | Weeks 6–10 | H3 and M5: RTO/RPO, restore test, uptime monitoring, second node or managed DB |
| 4. Policies and evidence | Weeks 2–10 (parallel) | Section 5, vendor register (M2), privacy inventory (M1), penetration test (M7) |
| 5. Readiness review, then audit | Weeks 10–12+ | Gap re-check; Type I; then a 3–6 month observation window for Type II |

## 7. Limits of this assessment

- I read code and config only; I did not run the stack or any scanner.
- I did not inspect the production VM, the ThunderID tenant (MFA, password policy, session lifetimes), the GitHub organization's member list, or any document kept outside this repo.
- The mapping to Trust Services Criteria is my judgement. An auditor may weigh evidence differently.
- This is a hackathon-origin codebase, and several items in `docs/security.md` already say "must be done before real production use". Those are consistent with this analysis.

## 8. Remediation status (branch `chore/soc2-hardening`)

| Item | Status |
|---|---|
| C3 scanning | **Done in repo:** `.github/workflows/security.yml` (govulncheck, npm audit at high, gitleaks, CodeQL for Go/JS/Python) and `.github/dependabot.yml`. GitHub-side secret scanning and push protection still need enabling in repo settings |
| Dependency vulnerabilities | **Fixed:** the `replace` block pinning x/net, x/text, x/sys and x/sync to 2025-era versions was removed and chi, pgx, grpc and OpenTelemetry were upgraded. govulncheck reported 12 module-level findings before and none after. Go moved to 1.26 (go.mod, all Dockerfiles, CI with `check-latest`). `go test ./...` passes and the order-service image builds |
| C4 default secrets/transport | **Guarded:** the production deploy preflight now refuses `sslmode=disable`, the default DB password, `local-*` M2M secrets and default Grafana/OpenSearch passwords. The VM `.env` must be updated before the next deploy |
| H5 audit enum drift | **Fixed** in `contracts/events/audit-event.schema.json` (adds `human`, `workflow`). The DB column is plain TEXT, so no migration was needed |
| M8 stray artefacts | **Fixed:** local tooling and screenshots added to `.gitignore` |
| Remaining Go vulnerabilities | 36 standard-library findings were reported against Go 1.26.0. They are fixed by a later 1.26 patch, which `golang:1.26-alpine` and `check-latest` pick up |
| C5 per-service DB roles | **Built, opt-in:** `database/roles.sh` creates six least-privilege logins, `database/tests/service_roles.sh` checks the grant matrix, and Compose uses `<SERVICE>_DATABASE_URL` with a fallback to `DATABASE_URL`. With none set, the resolved Compose config is identical to before. Tested on a throwaway Postgres 16: matrix passes, a deliberate extra grant is caught, re-running restores it, all six real service binaries start and report ready as their own login, and shared-service wrote an audit row. **Not yet switched on in production**; see `database/README.md` for the rollout. Note: earlier text said services never read another schema; `order-service` does (read-only) |
| Still open | C1 branch protection, C2 single release branch and environment reviewers, C6 policies, H1 MFA, H2 encryption at rest, H3 DR, H4 retention, H6 public repo, H7 persistent approvals, M1–M7. react-router has 2 moderate advisories that need a breaking v7 upgrade |
