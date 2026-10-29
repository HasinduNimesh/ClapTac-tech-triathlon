# Store manager product search

Screenshots of the actual `NewOrderPage` inside `StoreManagerLayout`, rendered with a temporary Vite preview and captured using Playwright/Chrome. Authentication and API responses are mocked. Products come from the repository's Fresh reference catalog; the account, notifications, and draft order are sample data. These are UI examples, not production orders or live backend verification.

| Screenshot | What it demonstrates |
| --- | --- |
| `01-search-ready.png` | Search above the product picker, with an available-product count. |
| `02-filtered-and-added.png` | Searching for rice narrows seven eligible ambient products to one; four bags are already in the draft and reflected in the review totals. |
| `03-no-matches.png` | An unmatched query shows guidance, disables product selection/addition, and preserves the existing draft line. |
| `04-mobile-search.png` | Search, clear action, result count, and picker at a 390 px viewport. |

Desktop captures use a 1440 px viewport. Browser checks covered widths of 1440, 1280, 1024, 768, 390, and 320 px, without horizontal page overflow. The item table scrolls within the form on narrow screens.

## Reproduce in the app

1. Sign in as a store manager and open **Place Orders**.
2. In **Items to deliver**, search by a product's name, code, family, size, or pack. Multiple words can match in any order.
3. Choose a matching product, enter the number of boxes, and select **Add item**.
4. Try a query with no matches. The existing order items should remain unchanged and **Add item** should be disabled.
5. Select **Clear search** to restore eligible products. Search stays limited to the selected goods type and excludes products already at the quantity limit.

Additional local browser checks used a synthetic 500-product catalog, exercised keyboard Enter safety and clear-button focus, checked temperature and quantity restrictions, verified a mocked submission's product/quantity payload and reset, and simulated catalog unavailability.
