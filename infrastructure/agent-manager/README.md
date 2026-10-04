# Agent manager

Records what each agent did during a request and **why**, and shows it in a small web page. It is infrastructure, not a business service: no business data, no database, and agents only push to it.

```bash
make compose-agent-traces        # whole stack + the manager, agents pointed at it
open http://localhost:8085       # the viewer (bound to 127.0.0.1)
```

Or by hand: `AGENT_TRACE_URL=http://agent-manager:8085 docker compose --profile agent-traces up --build`. Without `AGENT_TRACE_URL` the agents do not trace at all, and a missing or slow manager never affects an agent request (traces are queued, bounded, and dropped when the queue is full).

## What is recorded

One **trace** per agent request, made of ordered **steps**. Every step has a `reason`.

| Agent | Operations | Where the reasons come from |
| --- | --- | --- |
| `orchestrator` | `chat`, `approval.propose`, `approval.decide`, `workflow.draft` | Rules the code applied (allowlist, schema check, "sensitive tool so held for approval", "deterministic result returned as-is"), plus the rationale the model wrote next to a tool call |
| `order-assistant` | `draft` | Code decisions after the model transcribes the text: which product matched, why something became a question, which earlier order was picked |
| `dashboard-assistant` | `draft` | Why cards were kept or dropped by the fixed card list, why a question was asked |

Step kinds: `decision` (a rule applied), `llm` (a model call), `tool` (a business-service call), `guardrail` (validation/authorization), `approval`, `error`. Trace status: `ok`, `pending_approval`, `denied`, `error`.

Two honest limits:

- A model's reason is **what the model said**, not a verified cause. The step says "Model's stated rationale: …", or "gave no rationale" when it did not, so the viewer never invents one. The orchestrator's system prompt asks for one short sentence before a tool call.
- The assistants (A1/A2) use a forced function call, so the model gives no rationale there. Their reasons are the deterministic code decisions, which is where the choices are actually made.

## What is deliberately not recorded

Bearer tokens, provider keys, the raw request text, and tool output. The input is stored as a length and a SHA-256 only (the same approach as the audit log). Both the agents and the manager drop detail keys containing `token`, `secret`, `password`, `authorization`, `apikey`, `accesskey` or `signedurl`, and cap every field (400-character reasons, 100 steps per trace, 16 detail keys). A model's rationale can still paraphrase a request, so treat the viewer as staff-only.

## API

| Method | Path | Auth | |
| --- | --- | --- | --- |
| `POST` | `/api/v1/traces` | ingest token | Agents push one finished trace (`pkg/agenttrace` has the schema and a client) |
| `GET` | `/api/v1/traces?agent=&status=&q=&limit=` | view token | Summaries, newest first. `q` searches ids, user, tool names and reasons |
| `GET` | `/api/v1/traces/{id}` | view token | One full trace |
| `GET` | `/` | none | The viewer page (holds no data; its API calls send the view token) |
| `GET` | `/health/live`, `/health/ready` | none | |

## Configuration

| Variable | Default | |
| --- | --- | --- |
| `AGENT_TRACE_URL` | empty | On the **agents**: where to push. Empty turns tracing off |
| `AGENT_TRACE_INGEST_TOKEN` | empty | Bearer token agents must send. Set the same value on agents and manager |
| `AGENT_TRACE_VIEW_TOKEN` | empty | Bearer token to read. The viewer asks for it on a 401 and keeps it in `sessionStorage` |
| `ENVIRONMENT` | `local` | Anything but `local` makes both tokens **required**; the manager refuses to start without them |
| `AGENT_TRACE_MAX` | `1000` | Newest traces kept |
| `AGENT_TRACE_FILE` | `/data/traces.jsonl` in the image | JSON-lines persistence; empty keeps traces in memory only. Compacted on start |
| `HTTP_ADDR` | `:8085` | |

Compose publishes the manager on `127.0.0.1:8085` only and does not route it through NGINX, like Prometheus and Grafana. Do not expose it publicly without putting it behind the same access control as Grafana.

## Not done (on purpose, "nothing fancy")

No Kubernetes manifests or network-policy egress rules for agents to reach it yet. No retention beyond a count, no export, no live streaming, no link from a trace to the audit log other than the shared `correlation_id`. Traces are not distributed spans: the OpenTelemetry pipeline (`../observability/otel`) is separate and unchanged.
