# Agent 02 and Agent 03: priority tasks

Two deterministic, read-only agents turn existing operational records into a work queue. Open **Priority tasks** in the dispatcher or store-manager sidebar. No LLM, API key, scheduler, new database table, or migration is required for this PR. The recovery features depend on PR #176 and its rollout prerequisites.

## Rules and actions

| Agent | Trigger | Priority | Action / automatic removal |
| --- | --- | --- | --- |
| 02: dispatcher | Open chilled loading shortfall | Critical | Review the shortfall in Notifications; disappears when no short lines need a decision |
| 02 | Other open loading shortfall or unresolved dock alert | High | Review the loading decision / dock alert; disappears on source resolution |
| 02 | Today's open vehicle incident with no recovery case | High | Open Recovery for breakdowns, Live operations for other incidents |
| 02 | Pending recovery case | High | Review and approve; expired cases request new options; disappears after approval or supersession |
| 02 | Unallocated order with at least two recorded outlet deferrals | High | Review fair allocation in Planning; disappears when no longer unallocated |
| 02 | Other unallocated order | Normal | Allocate or defer in Planning |
| 03: store manager | Receipt due today or overdue, as reported by order-service | High | Confirm quantities / issues in Receipts; disappears after confirmation |
| 03 | Unexpired recovery options with no valid preference for the authorized outlet | High | Compare options and share preference; disappears on submission, expiry, supersession or approval |
| 03 | Other delivered / partially delivered order needing receipt | Normal | Confirm receipt |
| 03 | Deferred order | Normal | Review the reason and next run in the order timeline; remains while deferred |
| 03 | Active delivery with arrival expected within the next hour | Normal | Track delivery and prepare receiving space; latest prediction takes precedence over planned arrival |

Priorities are intentionally explainable. A breakdown alone does **not** imply chilled cargo or critical severity. Within each priority, dated tasks come before undated tasks, ordered by their source timestamp; stable task IDs break ties. The time field means receipt deadline, recovery expiry, estimated arrival, or incident/dock report time, as labelled on each card. An alternate time-first sort is available. Recovery cases suppress duplicate incident tasks. Exact repeated entity IDs are deduplicated.

The UI provides text-and-color priority badges, reasons, action links, normalized vehicle/order search, combined priority/type filters, 12-task pagination, a rules disclosure, and responsive layouts. Sinhala and Tamil labels are included. Vehicle identifiers render without underscores, and search tolerates spaces, underscores and hyphens.

## Scope, freshness and ownership

- Dispatcher tasks cover **today in Sri Lanka, across all accessible depots**, independently of the layout's depot selector. The scope is stated above the queue. This makes fleet-wide recovery work visible; it is not a depot-specific inbox.
- Store tasks use all authorized outlets, including older outstanding receipts and deferrals. Recovery cases are for today. Existing authenticated APIs enforce access; a second client-side outlet check prevents mixed records from being displayed accidentally. A store queue never calls dispatcher incident, loading or plan endpoints.
- Each refresh gets `/orders/cutoff` first and uses its server timestamp for date, expiry and arrival comparisons. No device wall clock is used by the task rules. Links carry the same delivery date; Planning, Live operations and Recovery now accept validated dates from those links.
- Fetch on open, every 30 seconds while visible, on the existing notification refresh event, or with **Refresh tasks**. Existing polling pauses in hidden tabs and backs off after failures. This is an on-screen agent, not a background notification service.
- Fetch sources independently. Keep earlier data for failed sources on the same operational day; failed tracking rows are retained if previously known. Display a prominent incomplete-data warning and mark affected tasks **Needs refresh**. First-load failures never claim that all work is finished. A clock failure retains the previous snapshot with a warning. A new identity/role remounts the queue and discards its snapshot.
- Only navigating to existing workflows is supported. There is no local “Done” checkbox that could conceal unfinished shared work, no automatic submission, and no bypass of the store-preference / dispatcher-approval boundary. The destination service remains authoritative when a person acts.

These are suggestions from the listed sources, not a complete operational checklist. Historical temperature audit events, messages without a resolution state, driver assignment, and a task-history/snooze/assignment system are outside this version. Arrival estimates can change. Order tracking currently uses the existing per-order API requests; very large store histories would benefit from a future server-side aggregate endpoint. Data from different services is not one transactional snapshot.

## Examples and evidence

The committed preview harness serves the **actual React task pages and role layouts with synthetic authenticated profiles and API responses**. The screenshots are UI evidence, not production data or evidence of live server writes. Pure rule tests verify the decisions separately.

1. Dispatcher: seven tasks, including one critical chilled shortfall, five high-priority decisions and one ordinary allocation. Recovery replaces the matching incident task. Vehicle search `wpnb7134` finds `WP NB 7134`.
2. Store: six tasks from its authorized outlet: overdue and due-today receipts, a recovery preference, an imminent arrival, a deferral and an ordinary receipt.
3. Source outage: fail the loading endpoint, retain both loading tasks, and flag their freshness. Fail one order's tracking endpoint and retain its previously known receipt task.
4. Resolution: update source fixtures to resolved/confirmed/approved states and refresh; the queue clears automatically.
5. Move the simulated browser clock to 2040; the store's six tasks and server date remain unchanged. Check mobile at 390 px without horizontal overflow.

```sh
cd apps/web
npm test
npm run build
node scripts/tasks-preview.mjs
# In another terminal; use your installed Playwright module and Chrome paths:
PLAYWRIGHT_MODULE=/absolute/path/to/playwright/index.mjs \
CHROME_PATH=/absolute/path/to/chrome \
node scripts/tasks-browser-check.mjs
```

Preview URLs: `http://127.0.0.1:4322/__preview?role=dispatch` and `http://127.0.0.1:4322/__preview?role=store`. Optional synthetic scenarios: `partial`, `tracking-failure`, `complete`, `empty`, `offline`. The development harness is never imported into the application bundle.

Screenshots are in [media/priority-todo-agents](media/priority-todo-agents). Browser checks cover filters/sorts, links, partial failures, source resolution, clock changes, runtime errors and mobile overflow.
