import { FormEvent, useEffect, useRef, useState } from "react";
import { Link, useNavigate, useSearchParams } from "react-router-dom";
import { useAuth } from "../auth/AuthContext";
import { useLocale } from "../i18n";
import { DashboardGrid } from "./DashboardCards";
import { useHelpersAvailable } from "../api/assistants";
import { TEMPLATES, moveCard } from "./dashboardRequest.mjs";
import { AssistantTurn, CARD_CATALOGUE, CardId, Dashboard, applyRequest, askAssistant, newDashboardId, saveToServer, useDashboards } from "./dashboards";
import { HelperUnavailableNote } from "./HelperUnavailableNote";
import { StoreManagerHero } from "./StoreManagerHero";
import { useOrderTrackings } from "./useOrderTrackings";

const SUGGESTIONS = ["Shortages this week", "Chilled deliveries", "Deferrals by reason", "Arrivals vs my window"];

export function CreateDashboardPage() {
  const { t, locale } = useLocale();
  const { profile, user } = useAuth();
  const token = user?.access_token || "";
  const helpers = useHelpersAvailable(token);
  const navigate = useNavigate();
  const [params] = useSearchParams();
  const outletId = profile?.outletIds?.[0] || "outlet";
  const { dashboards } = useDashboards();
  const existing = dashboards.find((d) => d.id === params.get("edit"));
  const { rows, loading } = useOrderTrackings();
  const now = new Date().toISOString();
  const [draft, setDraft] = useState<Dashboard>(existing || { id: newDashboardId(), name: "New dashboard", cards: [], createdAt: now, updatedAt: now, filter: "all" });
  const [turns, setTurns] = useState<AssistantTurn[]>([{ from: "assistant", text: `${t("Hi")}${profile?.userId ? ` ${profile.userId}` : ""}. ${t("What do you want to keep an eye on? I can build a dashboard from your orders, deliveries and receipts.")}` }]);
  const [message, setMessage] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [helperFailed, setHelperFailed] = useState(false);
  const loadedExisting = useRef(false);
  // Saved dashboards arrive from the server after the first render.
  useEffect(() => {
    if (existing && !loadedExisting.current) { loadedExisting.current = true; setDraft(existing); }
  }, [existing]);
  const log = useRef<HTMLDivElement>(null);
  useEffect(() => { log.current?.scrollTo({ top: log.current.scrollHeight }); }, [turns]);

  async function ask(text: string) {
    const value = text.trim();
    if (!value || busy) return;
    setBusy(true);
    setTurns((all) => [...all, { from: "manager", text: value }]);
    setMessage("");
    // The dashboard assistant reads the request when it is on; otherwise the
    // built-in keyword matcher keeps the screen working.
    const assisted = helpers?.dashboard ? await askAssistant(token, draft, value, locale) : null;
    if (helpers?.dashboard) setHelperFailed(!assisted);
    const result = assisted || applyRequest(draft, value);
    setDraft(result.draft);
    setTurns((all) => [...all, { from: "assistant", text: result.reply }]);
    setBusy(false);
  }
  // Off when the status says so (or a request just failed). Chat keeps working through the keyword
  // matcher and the cards can be ticked by hand, so a dashboard can always be built and saved.
  const helperOff = helpers !== null && (!helpers.dashboard || helperFailed);
  const toggleCard = (id: CardId) => setDraft({ ...draft, cards: draft.cards.includes(id) ? draft.cards.filter((c) => c !== id) : [...draft.cards, id], updatedAt: new Date().toISOString() });
  const touch = (next: Partial<Dashboard>) => setDraft((d) => ({ ...d, ...next, updatedAt: new Date().toISOString() }));
  const applyTemplate = (tpl: (typeof TEMPLATES)[number]) => {
    touch({ name: t(tpl.name), cards: tpl.cards as CardId[], filter: tpl.filter as Dashboard["filter"] });
    setTurns((all) => [...all, { from: "assistant", text: `${t("Started from")} "${t(tpl.name)}". ${t("Ask for a change, or save it when it looks right.")}` }]);
  };
  const cardTitle = (id: CardId) => t(CARD_CATALOGUE.find((c) => c.id === id)?.title || id);
  function send(e: FormEvent) { e.preventDefault(); void ask(message); }
  async function save() {
    if (!draft.cards.length || busy) return;
    setBusy(true);
    setError("");
    try {
      const saved = await saveToServer(token, draft);
      navigate(`/store-manager?dashboard=${encodeURIComponent(saved.id)}&saved=1`);
    } catch {
      setError(t("The dashboard could not be saved. Check your connection and try again."));
      setBusy(false);
    }
  }

  return (
    <>
      <StoreManagerHero compact title={existing ? t("Edit dashboard") : t("Create a new dashboard")} subtitle={t("Tell the assistant what you want to keep an eye on. It builds the dashboard from your outlet's orders and deliveries.")} />
      <div className="sm-page-body">
        <div className="dp-grid-2 dp-grid-2--even">
          <section className="dp-panel sm-chat" aria-label={t("Dashboard assistant")}>
            <div className="dp-panel-head" style={{ borderBottom: "1px solid #dee3ed" }}>
              <div className="dp-row"><span className="dp-avatar" aria-hidden="true">✦</span><div><h2 className="dp-panel-title" style={{ fontSize: "1rem" }}>{t("Dashboard assistant")}</h2><p className="dp-panel-sub" style={{ margin: 0 }}>{`${t("Uses only")} ${outletId} ${t("orders, deliveries and receipts")}`}</p></div></div>
            </div>
            <div className="sm-chat-log" ref={log} aria-live="polite">
              {turns.map((turn, i) => <p key={i} className={`sm-bubble sm-bubble--${turn.from}`}>{turn.text}</p>)}
              {turns.length === 1 && <div className="dp-chips">{SUGGESTIONS.map((s) => <button key={s} type="button" className="dp-chip" onClick={() => void ask(s)}>{t(s)}</button>)}</div>}
              {turns.length === 1 && !existing && (
                <div>
                  <p className="dp-section-label" style={{ margin: "12px 0 6px" }}>{t("Start from a template")}</p>
                  <div className="dp-chips">{TEMPLATES.map((tpl) => <button key={tpl.name} type="button" className="dp-chip" onClick={() => applyTemplate(tpl)}>{t(tpl.name)}</button>)}</div>
                </div>
              )}
            </div>
            {helperOff && (
              <div className="sm-helper-off">
                <HelperUnavailableNote />
                <fieldset className="sm-card-picker">
                  <legend>{t("Pick the cards by hand")}</legend>
                  {CARD_CATALOGUE.map((c) => (
                    <label key={c.id}><input type="checkbox" checked={draft.cards.includes(c.id)} onChange={() => toggleCard(c.id)} /> {t(c.title)}</label>
                  ))}
                </fieldset>
              </div>
            )}
            <form className="sm-chat-compose" onSubmit={send}>
              <input value={message} onChange={(e) => setMessage(e.target.value)} placeholder={t('Ask for a change, e.g. "add deferrals by reason"')} aria-label={t("Message the dashboard assistant")} maxLength={300} />
              <button type="submit" className="dp-btn" disabled={!message.trim() || busy}>{t("Send")}</button>
            </form>
            <p className="muted" style={{ margin: "0 16px 12px", fontSize: "0.75rem" }}>{t("The assistant only reads your data. It never changes orders or receipts.")}</p>
          </section>
          <section className="dp-panel" aria-label={t("Live preview")}>
            <div className="dp-panel-head">
              <div>
                <p className="dp-section-label">{t("Live preview")}</p>
                <label className="visually-hidden" htmlFor="dash-name">{t("Dashboard name")}</label>
                <input id="dash-name" className="dp-input" style={{ marginTop: 6, fontWeight: 600 }} value={draft.name} onChange={(e) => setDraft({ ...draft, name: e.target.value })} maxLength={60} />
              </div>
              <span className="dp-tag dp-tag--green">{t("Draft")} · {draft.cards.length} {t("cards")}</span>
            </div>
            <div className="dp-panel-body">
              {draft.cards.length > 0 && (
                <ul className="sm-card-order" aria-label={t("Cards in this dashboard")}>
                  {draft.cards.map((id, i) => (
                    <li key={id}>
                      <span>{cardTitle(id)}</span>
                      <button type="button" className="dp-btn dp-btn--sm dp-btn--secondary" disabled={i === 0} aria-label={`${t("Move up")}: ${cardTitle(id)}`} onClick={() => touch({ cards: moveCard(draft.cards, id, -1) as CardId[] })}>↑</button>
                      <button type="button" className="dp-btn dp-btn--sm dp-btn--secondary" disabled={i === draft.cards.length - 1} aria-label={`${t("Move down")}: ${cardTitle(id)}`} onClick={() => touch({ cards: moveCard(draft.cards, id, 1) as CardId[] })}>↓</button>
                      <button type="button" className="dp-btn dp-btn--sm dp-btn--secondary" aria-label={`${t("Remove")}: ${cardTitle(id)}`} onClick={() => touch({ cards: draft.cards.filter((c) => c !== id) })}>✕</button>
                    </li>
                  ))}
                </ul>
              )}
              {draft.cards.length === 0 ? <p className="dp-note">{t("Your dashboard appears here as you describe it. Try one of the suggestions.")}</p> : loading ? <p className="muted" role="status">{t("Loading your orders…")}</p> : <DashboardGrid dashboard={draft} rows={rows} />}
              {error && <p className="status-bad" role="alert">{error}</p>}
              <div className="dp-row" style={{ justifyContent: "flex-end", marginTop: 20 }}>
                <Link to="/store-manager" className="dp-btn dp-btn--secondary">{t("Cancel")}</Link>
                <button type="button" className="dp-btn" disabled={!draft.cards.length || !draft.name.trim() || busy} onClick={() => void save()}>{t("Save dashboard")}</button>
              </div>
            </div>
          </section>
        </div>
      </div>
    </>
  );
}
