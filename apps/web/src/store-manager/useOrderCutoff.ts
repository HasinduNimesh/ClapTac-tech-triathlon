import { useApi } from "../dispatcher/useApi";

/** The daily order cutoff the order service applies, as HH:MM Sri Lanka time; undefined until it is read (or if it cannot be). */
export function useOrderCutoff(): string | undefined {
  const { data } = useApi<{ cutoff: { localTime: string } }>("/orders/cutoff");
  return data?.cutoff?.localTime;
}
