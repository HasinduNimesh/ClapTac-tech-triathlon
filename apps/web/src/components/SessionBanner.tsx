import { useState } from "react";
import { useAuth } from "../auth/AuthContext";
import { useLocale } from "../i18n";
import "./sessionBanner.css";

/** Warns before a web sign-in ends and says plainly when it has, with one button to carry on. */
export function SessionBanner() {
  const { user, sessionPhase, sessionMinutesLeft, renewSession } = useAuth();
  const { t } = useLocale();
  const [busy, setBusy] = useState(false);
  if (!user || sessionPhase === "active") return null;
  const expired = sessionPhase === "expired";
  const carryOn = async () => {
    setBusy(true);
    try { await renewSession(); } finally { setBusy(false); }
  };
  return (
    <div className={`session-banner${expired ? " session-banner--ended" : ""}`} role={expired ? "alert" : "status"}>
      <p>
        {expired
          ? t("Your session has ended. Sign in again to carry on. You will come back to this page, but anything you had not saved is lost.")
          : t("Your session ends in {minutes} minutes. Finish and save what you are doing, or stay signed in.").replace("{minutes}", String(sessionMinutesLeft))}
      </p>
      <button type="button" onClick={() => void carryOn()} disabled={busy}>
        {busy ? t("Working…") : expired ? t("Sign in again") : t("Stay signed in")}
      </button>
    </div>
  );
}
