export type BellItem = {
  key: string;
  kind?: string;
  tone?: "red" | "amber" | "cool" | "primary" | "muted";
  tag?: string;
  title: string;
  text?: string;
  action?: string;
  to?: string;
  href?: string;
  readable?: boolean;
  [extra: string]: unknown;
};
export function unreadItems<T extends BellItem>(items: T[] | undefined, readKeys?: Set<string> | string[]): T[];
export function countUnread(items: BellItem[] | undefined, readKeys?: Set<string> | string[]): number;
export function badgeText(count: number): string;
export function bellLabel(count: number, t?: (source: string) => string): string;
