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
