# A3 and A4: testing and screen recording

## Implemented scope

A3 recognizes two explicit habits: the same aggregate order on three matching weekdays in 28 days, and a dispatcher manually flagging repeat-deferred outlets on three distinct days in 14 days. It reads only the person's own activity. Yes performs one prefill or personal review flag; No skips the occurrence; two No answers suppress the pattern; Don't ask again suppresses immediately. Feedback and the global preference persist in PostgreSQL. After the third Yes, the card offers an A4 draft. The helper is absent for loaders and drivers in this release.

A4 accepts the weekly deferred-order example in natural language and exposes an editable When / Check / Do definition, side-effect-free test, Turn on, Pause, Resume, Delete, Notifications and run history. The exact example works without an LLM using a deliberately narrow template parser. When configured, the existing LLM provider can interpret other phrasings of the supported weekly list; unsupported conditions ask for clarification. A3 can also hand over a weekly order-prefill or personal priority-review workflow. The structured builder remains available without the agent.

Dispatcher flags are personal seven-day review markers visible on Planning and My automations. They do not alter fairness scores, publish plans, or reassign orders. Repeat-deferred means at least two recorded deferral dates in 28 days, not a claim about consecutive failed deliveries. Orders currently contain aggregate units/weight/volume/temperature, not product lines.

## Start locally

From the repository root:

```sh
docker compose up -d --build
python3 scripts/demo-automations.py seed
```

Open http://localhost. Local accounts: `store-manager / waypoint` and `dispatcher / waypoint`.

Use a disposable local database for the fixture script. It refuses a running shared-service whose `ENVIRONMENT` is not `local`. `seed` creates explicitly synthetic history, initializes two earlier Yes responses, resets the two demo habit patterns and their personal review flags, and enables the helper for those accounts. It does not submit orders or publish plans. Records use `DEMO-A4-…` identifiers; My automations shows a demo-history banner. These fixtures are for demonstration, not evidence of real user activity. Re-running `seed` is the reset between recordings; audit history remains append-only.

If the stack already existed before this feature, re-run the migration job before rebuilding services:

```sh
docker compose run --rm migrate
docker compose up -d --build
```

## Suggested recording: 3–5 minutes

### 1. A3 store habit and human control

1. Run the seed command, then sign in as **store-manager**.
2. Show the corner **A3 · Habit helper** card. It explains three matching weekly orders.
3. Click **Yes, do it once**.
4. The order form opens with **30 units / 150 kg / 0.42 m³ / ambient**. Point out that the date is still reviewed and **Submit Order** has not been clicked.
5. Show **You've said Yes 3 times**. Two prior Yes answers came from the labelled fixture history; this click is the third.
6. Click **Review automation in A4**. A weekly order-draft workflow appears. It is not active. Show When / Check / Do, then Cancel if recording the separate deferral example next.

### 2. A4 describe, test and activate

1. Open **My automations → + New automation**.
2. Use the prefilled sentence: `Every Friday at 3 PM, if any of my orders are still deferred, send me a list.`
3. Click **Build draft**. Review Friday / 15:00 / Asia-Colombo and the in-app notification action.
4. Leave **Previous scheduled occurrence** selected and click **Test workflow**. The fixture's two `OUT034` orders appear. This performs no action.
5. Change the day or time and show that **Turn on** becomes disabled until the revised draft is tested again.
6. Test again and click **Turn on**. Show the active workflow and next scheduled time.

### 3. Show actual execution without waiting until Friday

With the workflow active, run this in a terminal:

```sh
python3 scripts/demo-automations.py run-due
```

This moves the next due time for active automations belonging to the two demo accounts to now. The normal production worker executes them within approximately 15 seconds. It is not an AI call or a fabricated UI result. Notifications and Recent runs refresh automatically every 15 seconds.

1. Show the deferred list under **From your automations**, or open the store's **Notifications** page.
2. Show the **completed** entry under **Recent runs**.
3. Click **Pause**, run `run-due` again and show that no additional notification appears for the paused workflow.
4. Show **Resume**, then **Delete**. Delete removes it from active workflows and retains audit/run evidence.

### 4. Dispatcher habit

1. Sign out; sign in as **dispatcher**.
2. Show the A3 repeat-deferral card and click **Yes, do it once**.
3. Open **Planning** or **My automations** and show **Flagged for review** beside the outlet.
4. Explain that this is a personal planning review flag; the deterministic allocation score and confirmed plans remain unchanged.
5. Show the A4 handoff for a weekly personal review list if useful.

### Optional: demonstrate suppression

```sh
python3 scripts/demo-automations.py suppression
```

This seeds one prior No for each demo pattern and clears that day's demo suggestion. Reload, click **No**, then reload again: the second No persists and no card returns. Run `seed` to reset for another recording. Separately, **Don't ask again** suppresses immediately. The Settings checkbox turns off A3 without pausing saved A4 workflows.

## Automated verification

Docker must be running. The new integration tests intentionally fail if PostgreSQL cannot start; they do not silently skip.

```sh
go test -count=1 ./pkg/automation ./services/shared-service/internal/automations ./services/planning-service/internal/store ./services/agent-orchestrator/internal/handler
make verify
```

Coverage includes schedule timezone boundaries; restricted workflow vocabulary; ambiguity handling; scoped history/results; another user's preview, suggestion and lifecycle access; concurrent Yes deduplication; persistent two-No suppression; three-Yes promotion; dispatcher review flags; preview/activate/pause/delete; concurrent scheduled execution; and historical deferral resolution/transaction rollback.

Additional manual checks:

- Sign in as `store-manager-b / waypoint`: the OUT034 fixtures and notifications must not appear.
- Restart shared-service after activation: workflows and feedback remain saved.
- Leave LLM variables empty: the example and structured builder work; scheduled execution continues.
- Enter an unsupported request such as “send an SMS and submit the orders”: it must not create an executable workflow.
- With a fresh unseeded database, last week's test reports missing history. Choose **Current data** instead; the UI must not invent a historical result.
- Review the dispatcher audit screen for `HABIT_SUGGESTED`, `HABIT_ANSWERED`, `AUTOMATION_ACTIVATED`, `AUTOMATION_RUN`, and `AUTOMATION_STATUS_CHANGED`.

## Architecture and operational notes

- The agent orchestrator only drafts definitions over HTTP and still imports no database drivers.
- `shared-service/internal/automations` owns persistence, habits, scoped results, and a 15-second worker. All permissions come from the current shared profile. No browser token is stored for background use.
- The shared-service M2M identity needs the new **automations:read-internal** scope in a real identity provider. It calls planning's scoped internal snapshot endpoint. Shared code never queries the planning schema.
- Migration 0040 creates automation tables and a planning-owned history projection. Triggers record deferral/allocation changes transactionally. Previous-period tests only claim coverage after history collection began.
- Each due workflow is row-locked. Its run, notification, review flags, next due time and audit commit together. Failed upstream requests leave it due for retry and log `automation_worker_failed`; they do not create a successful empty result. Current owner authorization is checked again on each run.
- After an outage, one catch-up run evaluates current data and advances to the next future weekly slot; it does not flood the inbox with every missed week.
- Historical records survive deletion for auditability. Deleted workflows cannot run. Saved templates referencing an outlet the owner can no longer access are hidden and paused on attempted execution.
- Existing order audit publication is best effort. Missing audit events reduce A3's evidence; the detector does not invent missing actions. A transactional order-event outbox is future hardening.
- This release does not implement arbitrary event-triggered recipes, brand filters, external messaging, product-line orders, loader/driver habits, or general novel-pattern discovery.

## PR recording checklist

### Recorded walkthrough

[Watch the A3/A4 demo (6 minutes 16 seconds)](media/a3-a4/a3-a4-demo.mp4). This is a compressed MP4 copy of the supplied `vid.mov`; the original recording is unchanged. The walkthrough uses the labelled synthetic demo history described above.

| A3 habit suggestion | A4 historical preview | A4 execution evidence |
| --- | --- | --- |
| ![A3 suggests a repeated store order](media/a3-a4/a3-habit-suggestion.png) | ![A4 previews two deferred orders](media/a3-a4/a4-preview.png) | ![A4 notification and completed run](media/a3-a4/a4-execution.png) |

Attach the video to the PR description and state that it uses labelled demo history. Include A3 evidence → Yes → prefill → A4 handoff; A4 sentence → preview → Turn on → real worker notification; and Pause/Delete. Include the test command results. Do not record `.env`, provider keys, or bearer tokens.

## Verification recorded on 3 October 2026

`make verify` passed: Go tests (including the new PostgreSQL suites), build, vet, agent import guard, all 114 web tests, PWA build, and 13 Python checks. Browser verification covered store prefill without submission, third-Yes handoff, a two-order historical preview, activation, actual scheduler delivery, pause, dispatcher review flags, and second-No suppression after reload. The optional live LLM provider was not configured; the documented sentence used the supported template parser.
