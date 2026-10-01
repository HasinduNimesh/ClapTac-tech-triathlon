import { createContext, ReactNode, useContext, useEffect, useMemo, useState } from "react";
import { translate } from "./locale.mjs";

export type Locale = "en" | "si" | "ta";
type LocaleContextValue = { locale: Locale; setLocale: (locale: Locale) => void; t: (source: string) => string };
const LocaleContext = createContext<LocaleContextValue>({ locale: "en", setLocale: () => undefined, t: (s) => s });

export function LocaleProvider({ children }: { children: ReactNode }) {
  const [locale, setLocaleState] = useState<Locale>(() => {
    const saved = localStorage.getItem("waypoint.locale");
    return saved === "si" || saved === "ta" ? saved : "en";
  });
  const setLocale = (next: Locale) => { localStorage.setItem("waypoint.locale", next); setLocaleState(next); };
  useEffect(() => { document.documentElement.lang = locale === "si" ? "si-LK" : locale === "ta" ? "ta-LK" : "en"; }, [locale]);
  const value = useMemo(() => ({ locale, setLocale, t: (source: string) => translate(locale, source) }), [locale]);
  return <LocaleContext.Provider value={value}>{children}</LocaleContext.Provider>;
}
export function useLocale() { return useContext(LocaleContext); }
