# Guarded agent plane (Milestone 7)

```text
Human JWT
  ↓ same bearer token
Agent Orchestrator ── fixed tool allowlist ──> business service
  │                                           authn + RBAC + resource scope + business validation
  ├── configured OpenAI-compatible provider
  └── owner-bound 5-minute approvals for sensitive writes
```

The orchestrator resolves the application profile from Shared Service and exposes Dispatcher tools plus a smaller Store Manager set. Tools use fixed service URLs, HTTP methods, paths, argument schemas, response limits, and timeouts. There is no arbitrary URL, SQL, or shell tool. The orchestrator has no database-driver imports.

Read tools include profile, orders, order tracking, plans, deterministic constraint results and simulation, fleet, loading, delivery, and pending receipt status. Every request is delegated with the inbound human bearer token so each business service remains authoritative. Planning simulation calls the same deterministic allocator as plan generation but does not persist assignments or plan state. The assistant returns the planning service's structured result as verified data rather than asking the model to reinterpret constraint outcomes.

Sensitive tools are `CreateOrder`, `DeferOrder`, `ReassignAllocation`, and `ConfirmPlan`. The model can propose one of these actions; the orchestrator validates the allowlisted arguments and creates an expiring approval. It does not call the business service until the signed-in approval owner selects **Approve and execute**. A rejected, expired, replayed, or different-user approval cannot execute. After approval, the target public API receives the original human bearer token and rechecks its own RBAC and deterministic rules.

`LLM_BASE_URL`, `LLM_MODEL`, and optional `LLM_API_KEY` configure the OpenAI-compatible Chat Completions provider. With no base URL/model, the agent returns `agent_unavailable`; all other platform workflows remain independent. Provider output is bounded, secrets are not logged or added to prompts, tool output is marked as untrusted data, and each turn is limited to three calls. Agent requests, bounded tool names, approval decisions, failures, and request/tool latency have metrics. Audit events contain the tool name and argument hash, never bearer tokens or provider keys.

Approvals are held in bounded process memory for the five-minute expiry. Deployments that need approvals to survive restarts can replace this repository with a shared TTL-backed store without changing the approval interface.

## Agent traces

Audit events say *that* something happened; agent traces say *what the agent did and why*. The orchestrator and both assistants record one trace per request (ordered steps, each with a reason such as "sensitive tool, so held for approval", or the rationale the model gave) and push it best-effort to `infrastructure/agent-manager`, which stores the newest few hundred and shows them in a small viewer. Tracing is off unless `AGENT_TRACE_URL` is set, never blocks or fails an agent request, and holds no tokens, keys, raw request text or tool output (input is a length and hash). The agent tier still has no database; the manager is separate infrastructure. See [its README](../../infrastructure/agent-manager/README.md).

## Order and dashboard assistants (A1, A2)

`services/agent-assistants` is a small Python service (FastAPI + LangGraph, no LangChain model wrappers) in the same agent tier. It follows the same rules as the orchestrator: it has no database driver or `DATABASE_URL`, it validates the ThunderID JWT against JWKS, it resolves role and outlet from `shared-service /profiles/me`, and it reads business services only with the caller's own bearer token. NGINX and kGateway route `/api/v1/agent/order-assistant/`, `/api/v1/agent/dashboard-assistant/` and `/api/v1/agent/assistants/` to it; every other `/api/v1/agent` path stays on the orchestrator.

```text
A1  load_context (shared outlet brand + own orders) → read_text (LLM, one forced function) → match_products (code) → draft
A2  interpret (LLM, one forced function) → validate (fixed card vocabulary) → draft
```

- **A1 order assistant** (Store Manager, "Paste or type your order" on Place an Order). The model only records what was written (family, literal size, quantity, unit, "as usual", which earlier order). Code maps that to the brand catalog (`assistants/order_assistant/catalog.json`), converts bottles to cases, splits ambient and chilled into separate drafts, finds "same as last Tuesday" among the outlet's own orders, and turns anything unclear into a question (unknown product, two sizes, missing quantity). The draft fills the existing form; the manager still presses Submit Order, so the W1 cutoff and order rules are unchanged. Orders still store totals only, so "same as last week" copies totals and "as usual" asks for a quantity.
- **A2 dashboard assistant** (Store Manager, "Create new dashboard" from the Figma store-manager screens). Stateless: the browser sends the current draft each turn, so Cancel leaves nothing. The model can only return a name, an ordered list of the eight store-manager card ids (`deadlines`, `receipts`, `short`, `ontime`, `shortByWeek`, `deferrals`, `chilled`, `arrivals`) and an all/chilled/ambient filter; the spec holds no outlet or user ids. The web app computes every card from the person's own order trackings, and when the assistant is off the page falls back to its built-in keyword matcher. Saving is a separate human action to `shared-service /api/v1/shared/dashboards` (owner-scoped, versioned, audited, `dashboard:manage-own`).

Both endpoints return `submitted: false` / `saved: false`; neither can write. Without `LLM_BASE_URL`/`LLM_MODEL` they return `503 agent_unavailable`, `GET /api/v1/agent/assistants/status` reports them off, and the UI hides the helper. Audit log lines record a hash of the text and bounded counts, never the raw text or tokens. In Kubernetes the provider is outside the cluster, so add an egress rule for it when you configure one.
