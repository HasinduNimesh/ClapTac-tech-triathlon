import { useEffect, useRef, useState } from "react";
import { useLocale } from "../i18n";
import { OrderTextHelper } from "./OrderTextHelper";
import type { FormFill } from "./orderDraft.mjs";

/** Three-star "sparkles" mark used for the helper button. */
function SparklesIcon() {
  return (
    <svg width="24" height="24" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true" focusable="false">
      <path d="M10 2.5c.4 3.9 1.9 5.6 5.6 6.1-3.7.5-5.2 2.2-5.6 6.1-.4-3.9-1.9-5.6-5.6-6.1 3.7-.5 5.2-2.2 5.6-6.1Z" />
      <path d="M18 12.5c.25 2.2 1.1 3.1 3.2 3.4-2.1.3-2.95 1.2-3.2 3.4-.25-2.2-1.1-3.1-3.2-3.4 2.1-.3 2.95-1.2 3.2-3.4Z" />
      <path d="M6 15.5c.2 1.6.8 2.3 2.3 2.5-1.5.2-2.1.9-2.3 2.5-.2-1.6-.8-2.3-2.3-2.5 1.5-.2 2.1-.9 2.3-2.5Z" />
    </svg>
  );
}

/**
 * Round sparkle button on the side of Place an Order. It opens the
 * "Paste or type your order" helper in a side panel, so the order form stays
 * the first thing on the page. The panel sits outside the form, so Enter in
 * its fields can never submit an order.
 */
export function OrderHelperLauncher({ token, onFill }: { token: string; onFill: (fill: FormFill, neededBy: string | null) => void }) {
  const { t } = useLocale();
  const [open, setOpen] = useState(false);
  // Set when the helper reports it is switched off. The panel then shows one calm line, and the button
  // is dropped as soon as the panel is closed.
  const [off, setOff] = useState(false);
  const button = useRef<HTMLButtonElement>(null);

  useEffect(() => {
    if (!open) return;
    document.getElementById("order-text")?.focus();
    const onKey = (event: KeyboardEvent) => {
      if (event.key === "Escape") { setOpen(false); button.current?.focus(); }
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [open]);

  if (off && !open) return null;

  return (
    <>
      <button
        ref={button}
        type="button"
        className="sm-ai-fab"
        aria-label={t("Paste, type or speak your order")}
        title={t("Paste, type or speak your order")}
        aria-expanded={open}
        aria-controls="order-helper-panel"
        onClick={() => setOpen((value) => !value)}
      >
        <SparklesIcon />
      </button>
      <aside id="order-helper-panel" className={`sm-ai-drawer${open ? " open" : ""}`} aria-label={t("Paste, type or speak your order")} hidden={!open}>
        <button type="button" className="sm-ai-drawer-close" aria-label={t("Close")} onClick={() => { setOpen(false); button.current?.focus(); }}>×</button>
        <OrderTextHelper token={token} onUnavailable={() => setOff(true)} onFill={(fill, neededBy) => { onFill(fill, neededBy); setOpen(false); }} />
      </aside>
    </>
  );
}
