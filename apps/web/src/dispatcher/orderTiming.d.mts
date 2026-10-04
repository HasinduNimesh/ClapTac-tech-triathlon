export function splitByDeliveryDate(orders: { requestedDeliveryDate?: string }[] | undefined | null, today: string): { overdue: number; dueToday: number; later: number };
