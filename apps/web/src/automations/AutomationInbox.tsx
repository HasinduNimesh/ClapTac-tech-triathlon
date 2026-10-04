import { useLocale } from "../i18n";
import { useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { useAuth } from "../auth/AuthContext";
import { apiJSON } from "../api/client";
import { apiBase, Result } from "./types";
export function ResultView({ result }: { result: Result }) {
  const { t } = useLocale();
  return <div><p>{result.message}</p>{result.prefill && <p><strong>{result.prefill.orderUnits}{" "}{t("units")}</strong> · {result.prefill.orderWeightKg}{" "}{t("kg ·")}{" "}{result.prefill.orderVolumeM3} m³ · {result.prefill.temperatureRequirement}</p>}
    {result.items.length > 0 && <ul>{result.items.map(item => <li key={item.orderId}><strong>{item.outletId}</strong> · {item.orderId}{" "}{t("· run")}{" "}{item.date}</li>)}</ul>}
  </div>;
}
export function AutomationInbox({ refresh = 0, runs = false }: { refresh?: number; runs?: boolean }) {
  const { t } = useLocale();
  const { user, profile } = useAuth(); const navigate = useNavigate(); const [items, setItems] = useState<{ id: string; title: string; result: Result; createdAt: string }[]>([]); const [error, setError] = useState("");
  useEffect(() => { if (!user) return; let alive = true; const load = () => apiJSON<{ items: typeof items }>(`${apiBase}/${runs ? "runs" : "inbox"}`, user.access_token).then(data => { if (alive) {setItems(data.items);setError("");} }).catch(() => { if (alive) setError("Automation history could not be loaded."); }); void load(); const timer = window.setInterval(() => void load(), 15000); return () => { alive = false; clearInterval(timer); }; }, [user?.access_token, profile?.userId, refresh, runs]);
  return <section className="auto-panel"><div className="auto-eyebrow">{runs ? "EXECUTION HISTORY" : "IN-APP NOTIFICATIONS"}</div><h2>{runs ? "Recent runs" : "From your automations"}</h2>
    {error && <p role="alert">{error}</p>}{items.length === 0 && !error && <p className="muted">{runs ? "Runs appear here after a scheduled workflow executes." : "Your automated lists and order drafts will appear here."}</p>}
    {items.map(item => <article className="auto-inbox-item" key={item.id}><h3>{item.title}</h3><small>{new Date(item.createdAt).toLocaleString("en-GB", { timeZone: "Asia/Colombo" })}{" "}{t("· Sri Lanka")}</small><ResultView result={item.result} />
      {!runs && item.result.prefill && <button onClick={() => navigate("/store-manager/orders/new", { state: { habitPrefill: item.result.prefill, prefillOwner: profile!.userId } })}>{t("Open draft for review")}</button>}
    </article>)}
  </section>;
}
