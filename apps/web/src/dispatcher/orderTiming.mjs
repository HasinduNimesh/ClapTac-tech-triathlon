/**
 * How many waiting orders are overdue (their delivery date has passed), due today, or later.
 * `today` is the Sri Lanka calendar date as YYYY-MM-DD; dates compare as text.
 */
export function splitByDeliveryDate(orders, today) {
  let overdue = 0;
  let dueToday = 0;
  let later = 0;
  for (const order of orders || []) {
    const date = order && order.requestedDeliveryDate;
    if (!date) continue;
    if (date < today) overdue += 1;
    else if (date === today) dueToday += 1;
    else later += 1;
  }
  return { overdue, dueToday, later };
}
