export type Locale = "en" | "si" | "ta";
export const labels: Record<Locale, Record<string, string>>;
export function translate(locale: Locale, source: string): string;
