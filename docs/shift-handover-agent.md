# Agent 04: shift handover

Agent 04 prepares a deterministic handover brief for dispatchers and store managers from the priority-task sources introduced in #177. It needs no LLM or API key. Open **Shift handover** in either role's sidebar.

## Workflow

1. Choose the audience: dispatchers share a fleet-wide brief; store managers select one of their authorized outlets.
2. Enter a shift title and notes. **Generate handover brief** checks the existing operational sources and compiles outstanding tasks, reasons, action links and Critical / High / Normal counts. Refreshing the open work preserves the notes.
3. Review the two-column preview, then **Publish to next shift**. The report is saved in shared-service with the authenticated author's identity and the server publication timestamp.
4. An incoming person opens **Team handovers**, reviews a report, confirms they have read it, and acknowledges. The report records who received it and when. Its author cannot self-acknowledge.
5. Follow links to the current operational workflows. Acknowledgement means the brief was received; it does not complete tasks, submit orders or approve recovery plans.

Published content is immutable through the API. Corrections are a new brief, preserving the earlier report. History is paged in groups of 20, newest first, and refreshes every 30 seconds while visible. A report contains the outstanding work known at handover, not a live status board or a completed-work ledger. The screen labels this distinction and links to the current task queue.

## Generation, time and limits

Generation reuses Agent 02/03's deterministic task rules. It reads all sources anew and refuses to prepare a report if any required source failed; it does not publish an incomplete list as if all work had been checked. Store generation filters to the selected outlet before constructing the persisted payload. Raw source responses and other outlets' records are not included.

The source timestamp comes from `/orders/cutoff`. Browser freshness uses the existing server-anchored monotonic clock; server publication independently requires a snapshot at most five minutes old, with a 30-second future-clock tolerance. Changing a device clock cannot extend that publication window.

Reports support 500 tasks, a 100-character title and 4,000 characters of notes; the request body is bounded to 512 KiB. Oversized queues produce an explicit error rather than silent truncation. Task lists scroll within the brief so long reports remain usable. Titles, notes and references render as text, and saved action links are limited to the role's existing task workflow paths.

The browser prepares this **reviewed, authored snapshot** from authenticated APIs. The server validates identity, audience, structure, links and freshness, but does not independently re-query every service or certify the submitted task content. This is a team report, not authoritative operational state. Destination services continue enforcing their normal permissions and business rules.

Drafts remain in page memory until publication; changing store or leaving the page discards an unpublished draft. There is no background scheduler, automatic shift detection, recipient roster, task transfer, or email/message delivery in this version. All authorized members of the chosen audience can read the report. The next person chooses when to acknowledge it.

## Persistence and permissions

Migration **0081_shift_handovers.sql** adds `shared.shift_handovers`. shared-service owns every query; no service reads another service's database schema for this feature.

Endpoints:

- `GET /api/v1/shared/handovers?role=dispatcher&scope=fleet&offset=0`
- `GET /api/v1/shared/handovers?role=store&scope=OUT001&offset=0`
- `POST /api/v1/shared/handovers?role=…&scope=…` with `{id, snapshot}`
- `POST /api/v1/shared/handovers/{id}/acknowledge?role=…&scope=…`

The application profile's role and outlet membership are authoritative. Store managers cannot publish, list or acknowledge outside their authorized outlet, and dispatcher and store histories remain separate. Drivers, loaders and machine-only identities are not handover actors. Reads are `no-store`.

Publication uses a UUID retained across retries. An advisory transaction lock serializes requests with that ID; the same author, audience and content return the existing report. Reusing the ID with different content or identity returns 409. A committed report can still be retried after the original snapshot has aged. The UI freezes the submitted payload after an ambiguous failure, offers a safe retry and reconciles the publication ID against refreshed history.

Acknowledgement locks the report row. The first different authorized person wins; retries by that person succeed idempotently, and a competing actor gets 409. Publish/acknowledgement and their audit events commit in the same transaction. Audit entries contain identity and audience/count metadata, not copies of private shift notes or task details.

[OpenAPI contract](../contracts/openapi/handovers.yaml).

## Validation and screenshot examples

The committed preview serves the actual React screens and layouts with synthetic profiles and API fixtures. Its persistence is simulated in memory; real persistence, authorization, concurrent writes and transaction rollback are exercised separately with PostgreSQL 16 integration tests.

- Dispatcher example: seven open tasks, including one critical chilled shortfall. Outgoing staff add cargo-handover context, publish, and a different dispatcher acknowledges.
- Store example: six tasks for OUT001, including receipts and recovery preference. A different store manager receives it; switching to OUT002 clears the selected report and displays that outlet's separate history.
- Failure examples: incomplete source data blocks preparation; a response lost after publication retries to one saved report; known stale rejection unlocks the draft for refresh.
- Browser clock moved to 2040: task generation and snapshot freshness still use server time.
- Mobile example: 390 px, no horizontal overflow. Acknowledgement leaves all source tasks unchanged.

```sh
go test ./services/shared-service/... -count=1
cd apps/web
npm test
npm run build
node scripts/handover-preview.mjs
# Separate terminal, with your local Playwright and Chrome paths:
PLAYWRIGHT_MODULE=/absolute/path/to/playwright/index.mjs \
CHROME_PATH=/absolute/path/to/chrome \
node scripts/handover-browser-check.mjs
```

Preview: `http://127.0.0.1:4323/__preview?role=dispatch` or `?role=store`. Add `&person=incoming` for the receiving person. Synthetic scenarios include `partial`, `tracking-failure`, `stale`, and `lost-response`. Restart the preview server for a fresh in-memory history before re-running screenshot checks.

Screenshots: [docs/media/shift-handover-agent](media/shift-handover-agent).

## Rollout

Stacked on **#177** (`aeshan/priority-todo-agents`), which depends on #176 and the earlier stack. Apply migration **0081** first, then deploy **shared-service** and the **web app**. This feature adds no machine scope or external dependency. Earlier recovery rollout requirements remain in effect. Do not deploy the UI before the handover endpoints/table are ready.

Merge the preceding stack first. Retarget this PR to `main` once those commits reach it; rebase this feature commit onto updated `main` if preceding PRs were squash-merged or rebased. Do not merge into a temporary feature base.
