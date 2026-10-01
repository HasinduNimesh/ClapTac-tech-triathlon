# AI disclosure

AI tools assisted with interface design, implementation, code review, and documentation in this project. The application includes an optional Dispatcher/Store Manager assistant that uses a configurable OpenAI-compatible provider. It is unavailable by default when no provider is configured; the ordering, access checks, business rules, and deterministic allocation/constraint engine remain application code.

The assistant can read only its allowlisted service tools. Proposed sensitive writes remain drafts until the same human actor approves the exact arguments; the target business service rechecks authorization and constraints. The LLM cannot decide or overwrite planning feasibility. The product remains usable if the provider is absent.
