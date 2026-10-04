import { useLocale } from "../i18n";
import { useEffect, useState } from "react";
import { useAuth } from "../auth/AuthContext";
import { apiJSON } from "../api/client";
import { apiBase, Result } from "./types";
export function PriorityReviews() {
  const { t } = useLocale();
  const { user } = useAuth(); const [data, setData] = useState<{ items: Result["items"]; marked: string[] }>({ items: [], marked: [] }); const [error, setError] = useState(""); const [busy, setBusy] = useState(false);
  async function load() { if (user) try { setData(await apiJSON(`${apiBase}/priorities`, user.access_token));setError(""); } catch { setError("Personal review flags are unavailable."); } }
  useEffect(() => { void load(); const handler = () => void load(); window.addEventListener("waypoint-priorities-changed", handler); return () => window.removeEventListener("waypoint-priorities-changed", handler); }, [user?.access_token]);
  async function mark(outletId: string) {setBusy(true);try { await apiJSON(`${apiBase}/priorities`, user!.access_token, { method: "POST", body: JSON.stringify({ outletId }) }); await load(); } catch (e) { setError(String(e)); }finally{setBusy(false);} }
  const candidates = [...new Set(data.items.filter(i => i.count >= 2).map(i => i.outletId))];
  return <section className="auto-panel"><div className="auto-eyebrow">{t("PERSONAL PLANNING REVIEW")}</div><h2>{t("Repeat-deferred outlets")}</h2><p className="muted">{t("Flag an outlet for your next review. Flags expire after seven days and do not change the allocation score.")}</p>
    {error && <p role="alert">{error}</p>}{candidates.length === 0 && <p>{t("No outlets have two recorded deferral dates in the last four weeks.")}</p>}
    {candidates.map(id => <div className="auto-list-row" key={id}><strong>{id}</strong>{data.marked.includes(id) ? <span className="auto-status">{t("Flagged for review")}</span> : <button disabled={busy} onClick={() => void mark(id)}>{t("Flag for next review")}</button>}</div>)}
  </section>;
}
