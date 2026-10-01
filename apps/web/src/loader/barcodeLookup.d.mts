import type { LoadingOrder } from "../api/loading";
export function findLoadOrderByCode(orders: LoadingOrder[], rawValue: string): LoadingOrder | undefined;
export function resolveLoadOrderCode(
  orders: LoadingOrder[],
  rawValue: string,
  isLoading: boolean,
): { kind: "not_found" } | { kind: "not_loading" | "unresolved_shortfall" | "ready"; order: LoadingOrder };
