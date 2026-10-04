import { FormEvent, useEffect, useState } from "react";
import { apiFetch, apiJSON } from "../api/client";
import { useAuth } from "../auth/AuthContext";
import { useLocale } from "../i18n";
import { buildAuditSearchParams } from "./auditQuery.mjs";

type AuditEvent = { event_id: string; actor_id: string; action: string; resource_type: string; resource_id: string; timestamp: string; source: string; reason?: string; new_state?: Record<string, unknown> };
type SearchResult = { items: AuditEvent[]; total: number; limit: number; offset: number };
type KPIs = { from: string; to: string; totalEvents: number; plansGenerated: number; ordersDeferred: number; tripsStarted: number; tripsCompleted: number; deliveryOutcomes: number; syncConflicts: number; byAction: { action: string; count: number }[] };

export function AuditPage() {
  const { user } = useAuth();
  const { t } = useLocale();
  const token = user?.access_token || "";
  const [query, setQuery] = useState("");
  const [action, setAction] = useState("");
  const [resourceId, setResourceId] = useState("");
  const [resourceType, setResourceType] = useState("");
  const [actorId, setActorId] = useState("");
  const [from, setFrom] = useState("");
  const [to, setTo] = useState("");
  const [result, setResult] = useState<SearchResult | null>(null);
  const [kpis, setKpis] = useState<KPIs | null>(null);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);

  async function load(offset = 0) {
    if (!token) return;
    setLoading(true); setError("");
    let params: URLSearchParams;
    try {
      params = buildAuditSearchParams({ query, action, resourceType, resourceId, actorId, from, to }, offset);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
      setLoading(false);
      return;
    }
    try {
      const [events, summary] = await Promise.all([
        apiJSON<SearchResult>(`/shared/audit/events?${params}`, token),
        apiJSON<KPIs>("/shared/audit/kpis", token),
      ]);
      setResult(events); setKpis(summary);
    } catch (e) { setError(e instanceof Error ? e.message : "Audit data could not be loaded"); }
    finally { setLoading(false); }
  }
  async function exportCsv() {
    if (!token) return;
    setError("");
    try {
      const params = buildAuditSearchParams({ query, action, resourceType, resourceId, actorId, from, to }, 0);
      params.delete("limit"); params.delete("offset");
      const res = await apiFetch(`/shared/audit/export.csv?${params}`, token, { headers: { Accept: "text/csv" } });
      if (!res.ok) throw new Error(await res.text() || res.statusText);
      const url = URL.createObjectURL(await res.blob());
      const a = document.createElement("a");
      a.href = url; a.download = "audit-export.csv"; a.click();
      URL.revokeObjectURL(url);
    } catch (e) { setError(e instanceof Error ? e.message : "Audit export failed"); }
  }
  useEffect(() => { void load(); }, [token]);
  function submit(e: FormEvent) { e.preventDefault(); void load(0); }

  return <section>
    <h2>{t("Audit history and operational KPIs")}</h2>
    <p>{t("Dispatcher access · audit records are read-only. KPI totals cover the most recent seven days.")}</p>
    {error && <p className="status-bad" role="alert">{error}</p>}
    {kpis && <div className="grid">
      <article><h3>{t("Plans generated")}</h3><strong>{kpis.plansGenerated}</strong></article>
      <article><h3>{t("Orders deferred")}</h3><strong>{kpis.ordersDeferred}</strong></article>
      <article><h3>{t("Trips started / completed")}</h3><strong>{kpis.tripsStarted} / {kpis.tripsCompleted}</strong></article>
      <article><h3>{t("Delivery outcomes")}</h3><strong>{kpis.deliveryOutcomes}</strong></article>
      <article><h3>{t("Sync conflicts")}</h3><strong>{kpis.syncConflicts}</strong></article>
      <article><h3>{t("Total recorded events")}</h3><strong>{kpis.totalEvents}</strong></article>
    </div>}
    <h3>{t("Search audit events")}</h3>
    <form onSubmit={submit} className="row">
      <label>{t("Text")}<input aria-label={t("Text")} value={query} onChange={e => setQuery(e.target.value)} maxLength={160} placeholder={t("Actor, action, resource, source")} /></label>
      <label>{t("Action")}<input aria-label={t("Action")} value={action} onChange={e => setAction(e.target.value)} maxLength={100} placeholder={t("Exact action name")} /></label>
      <label>{t("Resource type")}<input aria-label={t("Resource type")} value={resourceType} onChange={e => setResourceType(e.target.value)} maxLength={80} /></label>
      <label>{t("Resource ID")}<input aria-label={t("Resource ID")} value={resourceId} onChange={e => setResourceId(e.target.value)} maxLength={120} /></label>
      <label>{t("Actor ID")}<input aria-label={t("Actor ID")} value={actorId} onChange={e => setActorId(e.target.value)} maxLength={120} /></label>
      <label>{t("From (Sri Lanka time)")}<input aria-label={t("From (Sri Lanka time)")} type="datetime-local" value={from} onChange={e => setFrom(e.target.value)} /></label>
      <label>{t("To (Sri Lanka time)")}<input aria-label={t("To (Sri Lanka time)")} type="datetime-local" value={to} onChange={e => setTo(e.target.value)} /></label>
      <button type="submit" disabled={loading}>{loading ? t("Searching…") : t("Search")}</button>
      <button type="button" onClick={() => void exportCsv()}>{t("Export CSV")}</button>
    </form>
    {result && <>
      <p>{result.total} {t("matching events · page")} {Math.floor(result.offset / result.limit) + 1}</p>
      <div className="table-scroll"><table><thead><tr><th>{t("Time")}</th><th>{t("Action")}</th><th>{t("Actor")}</th><th>{t("Resource")}</th><th>{t("Source")}</th><th>{t("Details")}</th></tr></thead>
        <tbody>{result.items.map(ev => <tr key={ev.event_id}><td>{new Date(ev.timestamp).toLocaleString()}</td><td>{ev.action}</td><td>{ev.actor_id || t("System")}</td><td>{ev.resource_type} {ev.resource_id}</td><td>{ev.source}</td><td>{ev.reason || JSON.stringify(ev.new_state || {})}</td></tr>)}</tbody></table></div>
      <div className="row"><button disabled={loading || result.offset <= 0} onClick={() => void load(Math.max(0, result.offset - result.limit))}>{t("Previous")}</button><button disabled={loading || result.offset + result.limit >= result.total} onClick={() => void load(result.offset + result.limit)}>{t("Next")}</button></div>
    </>}
  </section>;
}
