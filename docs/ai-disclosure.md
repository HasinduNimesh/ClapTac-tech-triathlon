# AI disclosure

AI tools assisted with interface design, implementation, code review, and documentation in this project. The application includes an optional Dispatcher/Store Manager assistant that uses a configurable OpenAI-compatible provider. It is unavailable by default when no provider is configured; the ordering, access checks, business rules, and deterministic allocation/constraint engine remain application code.

Where agents are used in the product: the Dispatcher/Store Manager chat assistant (agent-orchestrator), the Store Manager order text helper (A1, turns a pasted order into draft form values for the manager to check and submit) and the dashboard builder (A2, turns a plain request into a dashboard draft the person saves). A1 and A2 only produce drafts; product matching, quantities, totals, previous-order lookup and dashboard cards are computed by application code, and the screens work without them.

The assistant can read only its allowlisted service tools. Proposed sensitive writes remain drafts until the same human actor approves the exact arguments; the target business service rechecks authorization and constraints. The LLM cannot decide or overwrite planning feasibility. The product remains usable if the provider is absent.
