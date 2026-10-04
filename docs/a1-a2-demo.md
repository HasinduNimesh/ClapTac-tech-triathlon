# Order text helper (A1) and dashboard assistant (A2): demo and checks

Both helpers are agents behind the scenes, but the screens never call them that. Store managers see a box on **Place an Order** and the **Create new dashboard** page from the Figma store-manager screens.

## Run it

```bash
LLM_BASE_URL=https://your-openai-compatible-provider/v1 LLM_MODEL=your-model LLM_API_KEY=... docker compose up --build
```

Sign in at <http://localhost/> as `store-manager` / `waypoint` (outlet OUT034). Without `LLM_BASE_URL` and `LLM_MODEL` the order box is hidden, and Create new dashboard falls back to its built-in keyword matcher. Every other screen works the same.

The recording and screenshots in `docs/media/a1-a2/` were made against a local stand-in model that returns fixed function calls for the phrases below, because no provider key was configured. The rest of the path is real: sign-in, NGINX, the Python service, product matching, shared-service saving and the web pages.

## A1: Paste or type your order

1. Open **Place Orders** and type `rice 10 bags, oil 24 bottles, and yoghurt as usual` in **Paste or type your order**, then **Fill in for me**.
2. Rice becomes 10 bags and oil 24 bottles becomes 2 cases of 12. Yoghurt is marked **How many?** because past orders don't store item lines, so the helper asks instead of guessing. Enter 3 and **Add line**.
3. **Use for the order form** copies the totals (units, kg, m³, ambient or chilled) into the form. Nothing is sent until **Submit Order**, and the 4 PM cutoff still applies.
4. Try `2 crates of milk, the big ones`. It asks whether you mean Fresh milk 1 L or 500 ml.
5. Try `same as last time` after placing an order. It offers to copy that order's totals.

## A2: Create new dashboard

1. On the Dashboard, open **+ Create new dashboard**.
2. Send `what arrived short this week and which receipts I still need to confirm`. The live preview shows four cards (short this week, items short by week, receipts to confirm, report-by deadlines). The assistant asks one question: all goods or chilled only.
3. Send `put the report-by deadlines first`. The cards are reordered.
4. **Save dashboard** stores it in shared-service, so it follows the person to another device. It then appears in the **Dashboard** menu with **Edit with chat**. **Cancel** leaves nothing behind.

The assistant can only choose from the eight store-manager cards and an all/chilled/ambient filter. A saved dashboard has no outlet or user ids, and the page computes every card from your own orders, deliveries and receipts.

## Checks

```bash
./scripts/test-agent-assistants.sh            # 28 Python tests, including the three A1 test phrases
go test ./services/shared-service/...         # dashboard validation + Postgres owner-isolation test
(cd apps/web && npm test && npm run build)
```
