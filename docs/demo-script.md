# Judge walkthrough (6–8 minutes)

Use the local Compose stack and the four demo accounts (`store-manager`, `dispatcher`, `loader`, `driver`; password `waypoint`). The local identity shim is a development fixture. Begin with a fresh demo date and use only disposable local data.

1. **Store Manager (1 min):** sign in, check the outlet profile, create an order for the selected delivery date, and show the cutoff/confirmation state.
2. **Dispatcher (1.5 min):** open the order queue and planning workspace, generate a plan, show deterministic capacity/cooling/window constraints, defer an unserved order with a reason if needed, and confirm only a fully resolved plan.
3. **Loader (1 min):** start the assigned trip, show stop-aware loading, record a missing/damaged shortfall, then resolve all required load outcomes and mark the trip ready.
4. **Driver (1.5 min):** start the route, open a stop, record delivery outcome and proof. Use offline mode to queue work, then reconnect and show synchronization/conflict feedback.
5. **Store Manager (0.75 min):** track the order, confirm the received quantities, or report a discrepancy.
6. **Dispatcher (0.5 min):** show event-based loading/delivery and receipt issue visibility; do not present it as GPS tracking.
7. **Engineering (0.75 min):** show the service ownership diagram, `/metrics` or Grafana workflow panels, security/resource checks, and the optional approval-gated assistant.

Do not claim a step was browser-tested, a live LLM was connected, or public deployment exists unless separately verified. The current workspace does not contain a public URL or production cloud resources.
