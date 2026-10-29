# Dispatcher vehicle search and sorting

These screenshots show the actual `PlanningPage` and `FleetPage` inside `DispatcherLayout`, served by a temporary Vite preview and captured with Playwright/Chrome. Authentication, API responses, the order, and all 36 vehicles are synthetic fixtures. They are UI evidence, not production data or proof of live backend allocation.

| Screenshot | What it shows |
| --- | --- |
| `01-allocation-search.png` | Search by vehicle number, weight-capacity sorting, the selected vehicle, and the existing compatibility checks. |
| `02-allocation-no-matches.png` | An unmatched query clears the hidden selection and disables Apply adjustment. |
| `03-mobile-allocation.png` | Allocation controls stack within a 390 px viewport. |
| `04-fleet-filtered.png` | Number search combined with the Refrigerated filter and descending weight capacity; six vehicles match. |
| `05-fleet-number-sort.png` | The default natural vehicle-number ordering and readable fleet labels. |

Desktop screenshots use a 1440 px viewport. Both the allocation drawer and Fleet page were checked at 1440, 1024, 768, 390, and 320 px for horizontal overflow.

## Reproduce

1. Sign in as a dispatcher and open **Plan and allocate** with a draft plan containing an unallocated order.
2. Select **Assign** on an order. Search for all or part of a vehicle number, then choose a vehicle and trip.
3. Change **Sort vehicles by**. Number ordering is natural (2 before 10); weight and volume options show the largest capacity first. Sorting keeps the selected vehicle.
4. Try a query with no matches. The selected vehicle clears and Apply adjustment is disabled. Clear search restores the list and returns keyboard focus to search.
5. Open **Fleet**. Combine search and sorting with the existing depot, type, availability, and cooling filters. Clear filters restores the current workspace depot and removes the other filters.

Underscores are replaced by spaces in Planning and Fleet vehicle labels: `WP_CAB_2` displays as `WP CAB 2`. Searches accept either form, as well as `wp-cab-2` and `wpcab2`. IDs used in option values, requests, and data joins are preserved. A mocked allocation request was checked to contain `vehicleId: "WP_CAB_2"` after choosing the cleaned-up label.

Browser checks also covered Enter not submitting the allocation form, reopening the picker with search/selection reset, fleet pagination resetting on search/sort/filter changes, and vehicle-detail labels.
