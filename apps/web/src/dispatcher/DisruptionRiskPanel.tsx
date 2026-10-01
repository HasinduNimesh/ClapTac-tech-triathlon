import { FormEvent, useEffect, useState } from "react";
import { ApiError, apiJSON } from "../api/client";
import { DisruptionRisk } from "../api/planning";
import { useLocale } from "../i18n";

const blank = { scope: "DISTRICT", scopeKey: "", riskType: "HEAVY_RAIN", severity: "MEDIUM", summary: "", source: "Manual dispatcher report", sourceReference: "", confidence: "0.5" };

export function DisruptionRiskPanel({ date, token }: { date: string; token: string }) {
  const { t } = useLocale();
  const [items, setItems] = useState<DisruptionRisk[]>([]);
  const [draft, setDraft] = useState(blank);
  const [decisions, setDecisions] = useState<Record<string, { decision: string; severity: string; reason: string }>>({});
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  async function refresh() {
    try {
      const result = await apiJSON<{ items: DisruptionRisk[] }>(`/planning/disruption-risks?date=${encodeURIComponent(date)}`, token);
      setItems(result.items || []);
      setError("");
    } catch (e) {
      setError(e instanceof ApiError ? `${e.status}: ${e.message}` : String(e));
    }
  }

  useEffect(() => { void refresh(); }, [date, token]);

  async function createRisk(event: FormEvent) {
    event.preventDefault();
    setBusy(true);
    try {
      await apiJSON<DisruptionRisk>("/planning/disruption-risks", token, {
        method: "POST",
        body: JSON.stringify({ ...draft, deliveryDate: date, confidence: Number(draft.confidence) }),
      });
      setDraft(blank);
      await refresh();
    } catch (e) {
      setError(e instanceof ApiError ? `${e.status}: ${e.message}` : String(e));
    } finally {
      setBusy(false);
    }
  }

  async function applyDecision(event: FormEvent, risk: DisruptionRisk) {
    event.preventDefault();
    const choice = decisions[risk.id] || { decision: "ACKNOWLEDGED", severity: "", reason: "" };
    setBusy(true);
    try {
      await apiJSON<DisruptionRisk>(`/planning/disruption-risks/${encodeURIComponent(risk.id)}/override`, token, {
        method: "POST",
        body: JSON.stringify({ decision: choice.decision, severity: choice.decision === "OVERRIDE" ? choice.severity : "", reason: choice.reason }),
      });
      setDecisions(current => ({ ...current, [risk.id]: { ...choice, reason: "" } }));
      await refresh();
    } catch (e) {
      setError(e instanceof ApiError ? `${e.status}: ${e.message}` : String(e));
    } finally {
      setBusy(false);
    }
  }

  return <section className="card" aria-labelledby="disruption-risks-heading">
    <h3 id="disruption-risks-heading">{t("Road and weather disruption risks")} · {date}</h3>
    <p className="muted">{t("Date-specific advisory signals with source and confidence. They inform dispatcher review only; vehicle, depot, capacity, and delivery-window constraints remain authoritative.")}</p>
    {error && <p className="status-bad" role="alert">{error}</p>}
    {items.length === 0 ? <p>{t("No disruption risks are recorded for this date.")}</p> : <ul>
      {items.map(risk => {
        const choice = decisions[risk.id] || { decision: "ACKNOWLEDGED", severity: "", reason: "" };
        return <li key={risk.id} className="card">
          <strong>{risk.severity} · {risk.riskType} · {risk.scope} {risk.scopeKey}</strong>
          <p>{risk.summary}</p>
          <p className="muted">{t("Source")}: {risk.source}{risk.sourceReference ? ` · ${risk.sourceReference}` : ""} · {t("Confidence")}: {(risk.confidence * 100).toFixed(0)}% · {new Date(risk.createdAt).toLocaleString()}</p>
          {risk.overrideDecision && <p className="muted">{t("Latest dispatcher decision")}: {risk.overrideDecision}{risk.overrideSeverity ? ` · ${risk.overrideSeverity}` : ""} · {risk.overrideReason} · {risk.overriddenBy}</p>}
          <form onSubmit={event => void applyDecision(event, risk)} className="row">
            <label>{t("Decision")}<select value={choice.decision} onChange={event => setDecisions(current => ({ ...current, [risk.id]: { ...choice, decision: event.target.value } }))}>
              <option value="ACKNOWLEDGED">{t("Acknowledge")}</option><option value="OVERRIDE">{t("Override severity")}</option><option value="DISMISSED">{t("Dismiss signal")}</option>
            </select></label>
            {choice.decision === "OVERRIDE" && <label>{t("Severity override")}<select value={choice.severity} onChange={event => setDecisions(current => ({ ...current, [risk.id]: { ...choice, severity: event.target.value } }))}><option value="">—</option><option value="LOW">{t("low")}</option><option value="MEDIUM">{t("medium")}</option><option value="HIGH">{t("high")}</option></select></label>}
            <label>{t("Reason (required)")}<input value={choice.reason} minLength={8} maxLength={500} required onChange={event => setDecisions(current => ({ ...current, [risk.id]: { ...choice, reason: event.target.value } }))} /></label>
            <button disabled={busy}>{t("Save decision")}</button>
          </form>
        </li>;
      })}
    </ul>}
    <form onSubmit={event => void createRisk(event)} className="issue-form">
      <h4>{t("Record a disruption risk")}</h4>
      <div className="row">
        <label>{t("Scope")}<select value={draft.scope} onChange={event => setDraft(current => ({ ...current, scope: event.target.value }))}><option value="DEPOT">{t("Depot")}</option><option value="DISTRICT">{t("District")}</option><option value="ROUTE">{t("Route")}</option></select></label>
        <label>{t("Scope key")}<input value={draft.scopeKey} maxLength={120} required onChange={event => setDraft(current => ({ ...current, scopeKey: event.target.value }))} placeholder={t("For example: Colombo North")} /></label>
        <label>{t("Risk type")}<select value={draft.riskType} onChange={event => setDraft(current => ({ ...current, riskType: event.target.value }))}><option value="HEAVY_RAIN">{t("Heavy rain")}</option><option value="FLOODING">{t("Flooding")}</option><option value="LANDSLIDE">{t("Landslide")}</option><option value="ROAD_CLOSURE">{t("Road closure")}</option><option value="ROAD_DAMAGE">{t("Road damage")}</option><option value="OTHER">{t("Other")}</option></select></label>
        <label>{t("Severity")}<select value={draft.severity} onChange={event => setDraft(current => ({ ...current, severity: event.target.value }))}><option value="LOW">{t("low")}</option><option value="MEDIUM">{t("medium")}</option><option value="HIGH">{t("high")}</option></select></label>
      </div>
      <label>{t("Summary")}<textarea value={draft.summary} maxLength={500} required onChange={event => setDraft(current => ({ ...current, summary: event.target.value }))} /></label>
      <div className="row">
        <label>{t("Source / provenance")}<input value={draft.source} maxLength={120} required onChange={event => setDraft(current => ({ ...current, source: event.target.value }))} /></label>
        <label>{t("Source reference (optional)")}<input value={draft.sourceReference} maxLength={300} onChange={event => setDraft(current => ({ ...current, sourceReference: event.target.value }))} /></label>
        <label>{t("Confidence (0 to 1)")}<input type="number" min="0" max="1" step="0.01" required value={draft.confidence} onChange={event => setDraft(current => ({ ...current, confidence: event.target.value }))} /></label>
      </div>
      <button disabled={busy}>{t("Add date-specific risk")}</button>
    </form>
  </section>;
}
