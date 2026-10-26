export type OrderSort = "newest" | "oldest" | "delivery" | "reference";
export type OrderFilters = { sort?: OrderSort; stage?: string; goods?: string; query?: string };
type BrowsableOrder = {
  stage: string;
  order: { id: string; orderRef: string; temperatureRequirement: string; requestedDeliveryDate: string; createdAt?: string };
};
export const orderSortOptions: [OrderSort, string][];
export const orderStages: string[];
export function browseOrders<T extends BrowsableOrder>(rows: T[], options?: OrderFilters): T[];
