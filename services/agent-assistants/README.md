# agent-assistants

A1 order assistant and A2 dashboard assistant for the Store Manager. Python 3.12, FastAPI, LangGraph, httpx, PyJWT. No database. See [agent plane](../../docs/architecture/agent-plane.md) and [contract](../../contracts/openapi/assistants.yaml).

```bash
pip install -r requirements.txt
python -m unittest discover -s tests -t .
python -m assistants.app   # :8080, needs OIDC_*, SHARED_SERVICE_URL, ORDER_SERVICE_URL; LLM_* optional
```

- `assistants/order_assistant/` — catalog, deterministic resolver, graph
- `assistants/dashboard_assistant/` — fixed card vocabulary, graph
- `assistants/llm.py` — one forced function call to an OpenAI-compatible endpoint
