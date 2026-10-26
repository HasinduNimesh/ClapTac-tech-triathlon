import { FormEvent, useCallback, useEffect, useRef, useState } from "react";
import { ApiError, apiJSON } from "../api/client";
import { todayInSriLanka } from "../api/date.mjs";
import { useLocale } from "../i18n";
import { errorText, useToken } from "../dispatcher/useApi";
import { RecoveryCandidate, RecoveryCase } from "./types";
import "./recovery.css";

const vehicle = (id: string) => id.replace(/_/g, " ");
const time = (value: string) => new Intl.DateTimeFormat("en-GB", { timeZone: "Asia/Colombo", hour: "2-digit", minute: "2-digit" }).format(new Date(value));
const strategies = { single: "One replacement vehicle", split: "Share across vehicles", partial: "Recover priority deliveries", defer: "Defer remaining deliveries" };
type Incident = { id: string; vehicleId: string; date: string; type: string; status: string; description: string };

export function RecoveryPage({ dispatcher = false }: { dispatcher?: boolean }) {
  const { t } = useLocale();
  const token = useToken();
  const [date, setDate] = useState(todayInSriLanka);
  const [cases, setCases] = useState<RecoveryCase[]>([]);
  const [incidents, setIncidents] = useState<Incident[]>([]);
  const [planId, setPlanId] = useState("");
  const [incidentId, setIncidentId] = useState("");
  const [handoverMinutes, setHandoverMinutes] = useState(30);
  const [loaded, setLoaded] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [serverNow, setServerNow] = useState(0);
  const clock = useRef({ server: 0, monotonic: 0 });
  const request = useRef(0);
  const activeDate = useRef(date); activeDate.current = date;

  const refresh = useCallback(async () => {
    if (activeDate.current !== date) return;
    const current = ++request.current;
    try {
      const data = await apiJSON<{ items: RecoveryCase[]; serverNow: string }>(`/planning/recovery?date=${date}`, token);
      let nextIncidents: Incident[] = [], nextPlan = "";
      if (dispatcher) {
        const [fleet, plan] = await Promise.all([
          apiJSON<{ items: Incident[] }>("/fleet/incidents?openOnly=true", token),
          apiJSON<{ plan: { id: string; status: string } | null }>(`/planning/plans?date=${date}`, token).catch((e) => { if (e instanceof ApiError && e.status === 404) return { plan: null }; throw e; }),
        ]);
        nextIncidents = fleet.items.filter((i) => i.date === date && i.type === "breakdown" && i.status === "open");
        nextPlan = plan.plan?.status === "confirmed" ? plan.plan.id : "";
      }
      if (current !== request.current) return;
      setCases(data.items); setIncidents(nextIncidents); setPlanId(nextPlan);
      setIncidentId((id) => nextIncidents.some((i) => i.id === id) ? id : nextIncidents[0]?.id || "");
      clock.current = { server: Date.parse(data.serverNow), monotonic: performance.now() };
      setServerNow(clock.current.server); setLoaded(true);
    } catch (e) { if (current === request.current) { setError(errorText(e)); setLoaded(true); } }
  }, [date, dispatcher, token]);

  useEffect(() => {
    setLoaded(false); setCases([]); setError(""); setNotice(""); setPlanId(""); setIncidents([]);
    void refresh();
    const poll = window.setInterval(() => { void refresh(); }, 30_000);
    const tick = window.setInterval(() => { if (clock.current.server) setServerNow(clock.current.server + performance.now() - clock.current.monotonic); }, 1000);
    return () => { request.current++; clearInterval(poll); clearInterval(tick); };
  }, [refresh]);

  async function generate(event: FormEvent) {
    event.preventDefault(); setBusy(true); setError(""); setNotice("");
    try {
      const now = clock.current.server + performance.now() - clock.current.monotonic;
      await apiJSON("/planning/recovery", token, { method: "POST", body: JSON.stringify({ planId, incidentId, readyAt: new Date(now + handoverMinutes * 60_000).toISOString() }) });
      await refresh(); setNotice(t("Recovery options are ready. Stores can now share their preference."));
    } catch (e) { setError(errorText(e)); } finally { setBusy(false); }
  }

  return <div className="rc-page">
    <header className="rc-heading"><div><p className="rc-eyebrow">{t("DELIVERY CONTINUITY")}</p><h1>{t("Recovery workspace")}</h1><p>{dispatcher ? t("Turn a breakdown into a clear, coordinated next step.") : t("Review your delivery options and tell dispatch what works for your store.")}</p></div><label className="rc-date">{t("Delivery date")}<input type="date" disabled={busy} value={date} onChange={(e) => setDate(e.target.value)} /></label></header>
    <ol className="rc-steps" aria-label={t("Recovery workflow")}><li><b>1</b><span>{t("Breakdown reported")}<small>{t("Confirmed fleet incident")}</small></span></li><li><b>2</b><span>{t("Compare recovery options")}<small>{t("Store managers share a preference")}</small></span></li><li><b>3</b><span>{t("Dispatcher approves")}<small>{t("Fleet plan checked and published")}</small></span></li></ol>
    {error && <div className="rc-alert rc-alert--error" role="alert"><strong>{t("Recovery needs attention")}</strong><p>{error}</p><button type="button" className="rc-link" onClick={() => { setError(""); void refresh(); }}>{t("Refresh")}</button></div>}
    {notice && <div className="rc-alert rc-alert--success" role="status">{notice}</div>}
    {dispatcher && loaded && <form className="rc-setup" onSubmit={generate}>
      <div className="rc-setup-intro"><span className="rc-pulse" aria-hidden="true" /><div><h2>{t("Detected breakdowns")}</h2><p>{t("Open fleet reports are checked automatically. Choose one to prepare recovery.")}</p></div></div>
      {incidents.length ? <><div className="rc-setup-fields"><label>{t("Affected vehicle")}<select value={incidentId} onChange={(e) => setIncidentId(e.target.value)}>{incidents.map((i) => <option key={i.id} value={i.id}>{vehicle(i.vehicleId)} · {i.description}</option>)}</select></label><label>{t("Cargo ready at depot in")}<select value={handoverMinutes} onChange={(e) => setHandoverMinutes(Number(e.target.value))}>{[15, 30, 60, 90, 120].map((m) => <option key={m} value={m}>{m} {t("minutes")}</option>)}</select></label><button className="rc-primary" disabled={busy || !planId || !serverNow}>{busy ? t("Checking recovery options…") : t("Generate recovery options")}</button></div><p className="rc-caption">{t("Confirm the handover time with the crew. Only idle vehicles at the same depot are considered; active routes stay unchanged.")}</p>{!planId && <p role="status">{t("Publish the delivery plan before preparing recovery.")}</p>}</> : <p className="rc-empty-line">{t("No open breakdowns for this delivery date.")}</p>}
    </form>}
    {!loaded ? <p role="status">{t("Loading recovery options…")}</p> : cases.length === 0 ? <section className="rc-empty"><span aria-hidden="true">↗</span><h2>{t("No recovery plans to review")}</h2><p>{dispatcher ? t("Generate options from an open breakdown to get started.") : t("If a breakdown affects your orders, dispatch will share recovery options here.")}</p></section> : cases.map((c) => <RecoveryReview key={c.id} recovery={c} dispatcher={dispatcher} now={serverNow} onSaved={refresh} />)}
    <p className="rc-footnote">{t("Travel times are estimates. Capacity, cooling, access, fuel and store windows are checked before an option is offered.")}</p>
  </div>;
}

function RecoveryReview({ recovery: c, dispatcher, now, onSaved }: { recovery: RecoveryCase; dispatcher: boolean; now: number; onSaved: () => Promise<void> }) {
  const { t } = useLocale(); const token = useToken();
  const [selected, setSelected] = useState(c.selectedId || c.preferences[0]?.candidateId || c.candidates.find((o) => o.recommended)?.id || c.candidates[0]?.id || "");
  const [note, setNote] = useState(""); const [ready, setReady] = useState(false);
  const [busy, setBusy] = useState(false); const [error, setError] = useState(""); const [saved, setSaved] = useState("");
  const outlets = Array.from(new Map(c.candidates.flatMap((o) => o.moves.map((m) => [m.outletId, m.outletName] as const))).entries());
  const [outletId, setOutletId] = useState(outlets[0]?.[0] || "");
  const approved = c.status === "approved";
  const expired = now >= Date.parse(c.expiresAt);
  const option = c.candidates.find((o) => o.id === (approved ? c.selectedId : selected));
  const active = !approved && !expired && c.status === "pending";
  const preference = c.preferences.find((p) => p.outletId === outletId);

  async function submit(event: FormEvent) {
    event.preventDefault(); if (!option) return; setBusy(true); setError(""); setSaved("");
    try {
      await apiJSON(`/planning/recovery/${c.id}/${dispatcher ? "approve" : "preference"}`, token, { method: dispatcher ? "POST" : "PUT", body: JSON.stringify({ candidateId: option.id, note, readyConfirmed: ready, outletId }) });
      setSaved(dispatcher ? t("Recovery approved. The updated plan is published for loading and driver acknowledgement.") : t("Preference sent to dispatch. Your delivery changes only after dispatcher approval."));
      await onSaved();
    } catch (e) { setError(errorText(e)); } finally { setBusy(false); }
  }

  return <section className="rc-case" aria-label={`${t("Recovery for")} ${vehicle(c.incident.vehicleId)}`}>
    <header className="rc-case-header"><div><div className="rc-tags"><span className={`rc-badge ${approved ? "rc-badge--green" : "rc-badge--amber"}`}>{approved ? t("Approved") : expired ? t("Options expired") : t("Awaiting decision")}</span><span className="rc-meta">{c.depot} · {t("Plan version")} {approved ? c.publishedVersion : c.planVersion}</span></div><h2>{t("Recovery for")} {vehicle(c.incident.vehicleId)}</h2><p>{c.incident.description || t("A vehicle breakdown affects your deliveries.")}</p></div><div className="rc-handover"><small>{t("Depot handover")}</small><strong>{time(c.readyAt)}</strong><span>{t("Sri Lanka time")}</span></div></header>
    {expired && !approved && <div className="rc-alert rc-alert--warning">{dispatcher ? t("These options have expired. Generate fresh options above before approving.") : t("These options have expired. Dispatch needs to refresh them before you can choose.")}</div>}
    <div className="rc-comparison-heading"><h3>{dispatcher ? t("Compare fleet recovery plans") : t("Your store’s delivery options")}</h3><span>{dispatcher ? t("Recommendation considers the whole fleet") : t("Only your store’s orders are shown")}</span></div>
    <div className="rc-options" role="radiogroup" aria-label={t("Recovery options")}>
      {c.candidates.map((o, index) => <CandidateCard key={o.id} group={c.id} option={o} index={index} checked={o.id === option?.id} disabled={!active || busy} dispatcher={dispatcher} onSelect={() => { setSelected(o.id); setReady(false); setSaved(""); }} />)}
    </div>
    <p className="rc-recommendation"><span aria-hidden="true">✦</span>{t("Recommended from the evaluated options: protect chilled goods, recover more orders, then reduce delay, vehicles and distance.")}</p>
    {option && <details className="rc-details" open><summary>{t("Delivery details for the selected option")} <span>{option.moves.length} {t("orders")}</span></summary><div className="rc-table-wrap"><table><thead><tr><th>{t("Order / store")}</th><th>{t("Cargo")}</th><th>{t("Recovery vehicle")}</th><th>{t("Estimated arrival")}</th><th>{t("Change")}</th></tr></thead><tbody>{option.moves.map((m) => <tr key={m.orderId}><td><strong>{m.orderRef || m.orderId}</strong><small>{m.outletName || m.outletId}</small></td><td><span className={`rc-badge ${m.chilled ? "rc-badge--blue" : "rc-badge--neutral"}`}>{m.chilled ? t("Chilled") : t("Ambient")}</span></td><td>{m.deferred ? "—" : vehicle(m.vehicleId)}</td><td>{m.deferred ? t("Next run requested") : time(m.arrival)}</td><td>{m.deferred ? <span className="rc-deferred">{t("Deferred")}</span> : m.delayMinutes ? `+${m.delayMinutes} ${t("min")}` : t("On schedule")}</td></tr>)}</tbody></table></div>{option.deferred > 0 && <p className="rc-defer-note">{t("Deferred orders cannot meet the checked constraints. The next delivery date is a request, not a confirmed booking.")}</p>}</details>}
    {dispatcher && c.preferences.length > 0 && <aside className="rc-preferences"><h3>{t("Store preferences")}</h3>{c.preferences.map((p) => <div key={p.outletId}><strong>{outlets.find(([id]) => id === p.outletId)?.[1] || p.outletId}</strong><span>{t("Option")} {c.candidates.findIndex((o) => o.id === p.candidateId) + 1}</span><p>{p.note || t("No additional note")}</p></div>)}</aside>}
    {error && <div className="rc-alert rc-alert--error" role="alert">{error}</div>}
    {saved && <div className="rc-alert rc-alert--success" role="status">{saved}</div>}
    {approved ? <div className="rc-approved"><strong>{t("Recovery plan published")}</strong><p>{dispatcher ? c.approvalNote : t("Dispatch has approved the selected option. Follow your updated orders in tracking.")}</p><small>{t("Replacement trips still require loading, driver checkout and acknowledgement before departure.")}</small></div> : active && <form className="rc-decision" onSubmit={submit}>
      <div><h3>{dispatcher ? t("Review and approve") : t("Share your preference")}</h3><p>{dispatcher ? t("You approve the fleet-wide plan. Explain your choice, especially if it differs from a store’s preference.") : t("Your preference helps dispatch choose. It does not reassign vehicles.")}</p>{!dispatcher && outlets.length > 1 && <label>{t("Store")}<select value={outletId} onChange={(e) => setOutletId(e.target.value)}>{outlets.map(([id, name]) => <option key={id} value={id}>{name || id}</option>)}</select></label>}
        {!dispatcher && preference && <p className="rc-existing" role="status">{t("Saved preference")}: {t("Option")} {c.candidates.findIndex((o) => o.id === preference.candidateId) + 1} {preference.note && `· ${preference.note}`}</p>}
        <label>{dispatcher ? t("Decision note") : t("What matters to your store? (optional)")}<textarea value={note} onChange={(e) => setNote(e.target.value)} required={dispatcher} minLength={dispatcher ? 5 : undefined} maxLength={dispatcher ? 1000 : 500} rows={2} placeholder={dispatcher ? t("Explain the chosen trade-off and handover arrangements…") : t("For example: chilled stock is needed before the lunch rush…")} /></label>
        {dispatcher && <label className="rc-check"><input type="checkbox" checked={ready} onChange={(e) => setReady(e.target.checked)} required /><span>{t("I confirmed replacement drivers, vehicle readiness, cargo condition and depot handover by the stated time. The broken vehicle will be suspended, including if publication needs a retry.")}</span></label>}
      </div><div className="rc-submit"><button className="rc-primary" disabled={busy || !option || (dispatcher && (!ready || note.trim().length < 5))}>{busy ? t("Saving…") : dispatcher ? t("Approve and publish recovery") : t("Send preference")}</button><small>{dispatcher ? t("Current conditions are rechecked before publishing.") : t("Dispatcher approval is required.")}</small></div>
    </form>}
  </section>;
}

function CandidateCard({ group, option: o, index, checked, disabled, dispatcher, onSelect }: { group: string; option: RecoveryCandidate; index: number; checked: boolean; disabled: boolean; dispatcher: boolean; onSelect: () => void }) {
  const { t } = useLocale();
  return <label className={`rc-option ${checked ? "rc-option--selected" : ""}`}>
    <div className="rc-option-top"><span>{t("Option")} {index + 1}</span>{o.recommended && <span className="rc-badge rc-badge--green">✦ {t("Recommended")}</span>}<input type="radio" name={`recovery-option-${group}`} checked={checked} disabled={disabled} onChange={onSelect} aria-label={`${t("Option")} ${index + 1}: ${t(strategies[o.strategy])}`} /></div>
    <h4>{t(strategies[o.strategy])}</h4><p>{o.strategy === "split" ? t("Share the remaining load to reach stores sooner.") : o.strategy === "single" ? t("Keep the remaining deliveries together on one vehicle.") : o.strategy === "partial" ? t("Protect feasible deliveries and make the shortfall clear.") : t("No checked vehicle can serve the remaining orders.")}</p>
    <div className="rc-metrics"><div><strong>{o.recovered}<small> / {o.moves.length}</small></strong><span>{t("orders recovered")}</span></div><div><strong>{o.vehicles}</strong><span>{t("vehicles needed")}</span></div></div>
    <div className="rc-vehicle-list">{Array.from(new Set(o.moves.filter((m) => !m.deferred).map((m) => m.vehicleId))).map((id) => <span key={id}>{vehicle(id)}</span>)}</div>
    <div className="rc-option-bottom"><span className={o.deferred ? "rc-deferred" : "rc-positive"}>{o.deferred} {t("deferred")}</span><span>+{o.delayMinutes} {t("min total delay")}</span>{dispatcher && <span>{o.distanceKm} {t("km estimated")}</span>}</div>
  </label>;
}
