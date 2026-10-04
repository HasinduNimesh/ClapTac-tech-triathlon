import { useLocale } from "../i18n";
import { useEffect, useState } from "react";
import { useLocation, useNavigate } from "react-router-dom";
import { useAuth } from "../auth/AuthContext";
import { apiJSON } from "../api/client";
import { apiBase, fromHabit, Habit } from "./types";
import "./automations.css";

export function HabitHelper() {
  const { t } = useLocale();
  const { user, profile } = useAuth(); const navigate = useNavigate(); const location = useLocation();
  const [habit, setHabit] = useState<Habit | null>(null); const [accepted, setAccepted] = useState<Habit | null>(null);
  const [busy, setBusy] = useState(false); const [error, setError] = useState("");
  const role = profile?.roles?.[0]; const eligible = role === "STORE_MANAGER" || role === "DISPATCHER";
  useEffect(() => {
    setHabit(null); setAccepted(null); setError("");
    if (!user || !eligible) return;
    let active = true;
    async function load() { try { const data = await apiJSON<{ items: Habit[] }>(`${apiBase}/habits`, user!.access_token); if (active) setHabit(data.items[0] || null); } catch { /* Optional helper must not block operational screens. */ } }
    const onChange = () => { setAccepted(null); void load(); }; window.addEventListener("waypoint-habits-changed", onChange);
    void load(); const timer = window.setInterval(() => void load(), 60000);
    return () => { active = false; window.clearInterval(timer); window.removeEventListener("waypoint-habits-changed", onChange); };
  }, [user?.access_token, profile?.userId, eligible]);
  if (!eligible || (!habit && !accepted)) return null;
  const current = accepted || habit!;
  async function answer(response: string) {
    if (busy || !habit) return; setBusy(true); setError("");
    try {
      const result = await apiJSON<Habit>(`${apiBase}/habits/${habit.id}/respond`, user!.access_token, { method: "POST", body: JSON.stringify({ response }) });
      setHabit(null);
      if (response === "yes") {
        setAccepted(result);
        if (result.prefill) navigate("/store-manager/orders/new", { state: { habitPrefill: result.prefill, prefillOwner: profile!.userId } });
        window.dispatchEvent(new Event("waypoint-priorities-changed"));
      }
    } catch (e) { setError(String(e)); } finally { setBusy(false); }
  }
  return <aside className="habit-card" aria-label="Habit helper">
    <div className="auto-eyebrow">{t("A3 · HABIT HELPER")}</div>
    <h3>{accepted ? "Ready for your review" : "A familiar routine?"}</h3>
    <p>{accepted ? (accepted.prefill ? "Your order template is filled in. Review the date and quantities, then submit when ready." : "Added to your personal planning review list for seven days. Plan allocations are unchanged.") : current.evidence}</p>
    {error && <p role="alert" className="status-bad">{error}</p>}
    {!accepted ? <div className="auto-actions"><button disabled={busy} onClick={() => void answer("yes")}>{t("Yes, do it once")}</button><button disabled={busy} onClick={() => void answer("no")}>{t("No")}</button><button disabled={busy} className="auto-text" onClick={() => void answer("never")}>{t("Don't ask again")}</button></div> : <>
      {accepted.yesCount >= 3 && <><p><strong>{t("You've said Yes")}{" "}{accepted.yesCount}{" "}{t("times. Make this automatic?")}</strong></p><button onClick={() => { navigate(role === "STORE_MANAGER" ? "/store-manager/automations" : "/dispatcher/automations", { state: { draft: fromHabit(accepted), draftOwner: profile!.userId } }); setAccepted(null); }}>{t("Review automation in A4")}</button></>}
      <button className="auto-text" onClick={() => setAccepted(null)}>{t("Dismiss")}</button>
    </>}
    {!accepted && <small>{location.pathname.includes("automations") ? "Two No answers stop this pattern." : "You stay in control. Change preferences in My automations."}</small>}
  </aside>;
}
