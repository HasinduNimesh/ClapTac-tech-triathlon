import { FormEvent, useEffect, useRef, useState } from "react";
import { Link, useNavigate, useSearchParams } from "react-router-dom";
import { useAuth } from "../auth/AuthContext";
import { useLocale } from "../i18n";
import { DashboardGrid } from "./DashboardCards";
import { AssistantTurn, Dashboard, applyRequest, loadDashboards, newDashboardId, saveDashboards } from "./dashboards";
import { StoreManagerHero } from "./StoreManagerHero";
import { useOrderTrackings } from "./useOrderTrackings";

const SUGGESTIONS = ["Shortages this week", "Chilled deliveries", "Deferrals by reason", "Arrivals vs my window"];

export function CreateDashboardPage() {
  const { t } = useLocale();
  const { profile } = useAuth();
  const navigate = useNavigate();
  const [params] = useSearchParams();
  const userId = profile?.userId || "anonymous";
  const outletId = profile?.outletIds?.[0] || "outlet";
  const existing = loadDashboards(userId, outletId).find((d) => d.id === params.get("edit"));
  const { rows, loading } = useOrderTrackings();
  const now = new Date().toISOString();
  const [draft, setDraft] = useState<Dashboard>(existing || { id: newDashboardId(), name: "New dashboard", cards: [], createdAt: now, updatedAt: now, filter: "all" });
  const [turns, setTurns] = useState<AssistantTurn[]>([{ from: "assistant", text: `${t("Hi")}${profile?.userId ? ` ${profile.userId}` : ""}. ${t("What do you want to keep an eye on? I can build a dashboard from your orders, deliveries and receipts.")}` }]);
  const [message, setMessage] = useState("");
  const log = useRef<HTMLDivElement>(null);
  useEffect(() => { log.current?.scrollTo({ top: log.current.scrollHeight }); }, [turns]);

  function ask(text: string) {
    const value = text.trim();
    if (!value) return;
    const result = applyRequest(draft, value);
    setDraft(result.draft);
    setTurns((all) => [...all, { from: "manager", text: value }, { from: "assistant", text: result.reply }]);
    setMessage("");
  }
  function send(e: FormEvent) { e.preventDefault(); ask(message); }
  function save() {
    if (!draft.cards.length) return;
    const all = loadDashboards(userId, outletId).filter((d) => d.id !== draft.id);
    saveDashboards(userId, outletId, [...all, { ...draft, updatedAt: new Date().toISOString() }]);
    navigate(`/store-manager?dashboard=${encodeURIComponent(draft.id)}&saved=1`);
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
              {turns.length === 1 && <div className="dp-chips">{SUGGESTIONS.map((s) => <button key={s} type="button" className="dp-chip" onClick={() => ask(s)}>{t(s)}</button>)}</div>}
            </div>
            <form className="sm-chat-compose" onSubmit={send}>
              <input value={message} onChange={(e) => setMessage(e.target.value)} placeholder={t('Ask for a change, e.g. "add deferrals by reason"')} aria-label={t("Message the dashboard assistant")} maxLength={300} />
              <button type="submit" className="dp-btn" disabled={!message.trim()}>{t("Send")}</button>
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
              {draft.cards.length === 0 ? <p className="dp-note">{t("Your dashboard appears here as you describe it. Try one of the suggestions.")}</p> : loading ? <p className="muted" role="status">{t("Loading your orders…")}</p> : <DashboardGrid dashboard={draft} rows={rows} />}
              <div className="dp-row" style={{ justifyContent: "flex-end", marginTop: 20 }}>
                <Link to="/store-manager" className="dp-btn dp-btn--secondary">{t("Cancel")}</Link>
                <button type="button" className="dp-btn" disabled={!draft.cards.length || !draft.name.trim()} onClick={save}>{t("Save dashboard")}</button>
              </div>
            </div>
          </section>
        </div>
      </div>
    </>
  );
}
