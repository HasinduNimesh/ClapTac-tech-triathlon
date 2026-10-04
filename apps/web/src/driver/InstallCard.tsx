import { useEffect, useState } from "react";
import { useLocale } from "../i18n";
import { INSTALL_INTRO_KEY, installCardState, manualInstallSteps } from "./installPrompt.mjs";

type InstallPromptEvent = Event & { prompt: () => Promise<void>; userChoice: Promise<{ outcome: string }> };

function readDismissed() { try { return window.localStorage.getItem(INSTALL_INTRO_KEY) === "1"; } catch { return false; } }
function saveDismissed() { try { window.localStorage.setItem(INSTALL_INTRO_KEY, "1"); } catch { /* shown again next visit */ } }
const isStandalone = () => window.matchMedia?.("(display-mode: standalone)").matches || (navigator as Navigator & { standalone?: boolean }).standalone === true;

/** DR-8: first-run card asking the driver to put Waypoint on the home screen, and saying what stays offline. */
export function InstallCard() {
  const { t } = useLocale();
  const [dismissed, setDismissed] = useState(readDismissed);
  const [promptEvent, setPromptEvent] = useState<InstallPromptEvent | null>(null);
  const [standalone, setStandalone] = useState(isStandalone);

  useEffect(() => {
    const onPrompt = (event: Event) => { event.preventDefault(); setPromptEvent(event as InstallPromptEvent); };
    const onInstalled = () => { setStandalone(true); saveDismissed(); };
    window.addEventListener("beforeinstallprompt", onPrompt);
    window.addEventListener("appinstalled", onInstalled);
    return () => { window.removeEventListener("beforeinstallprompt", onPrompt); window.removeEventListener("appinstalled", onInstalled); };
  }, []);

  const state = installCardState({ standalone, dismissed, canPrompt: Boolean(promptEvent) });
  if (state === "hidden") return null;
  const close = () => { saveDismissed(); setDismissed(true); };
  async function install() {
    if (!promptEvent) return;
    await promptEvent.prompt();
    const choice = await promptEvent.userChoice.catch(() => ({ outcome: "dismissed" }));
    setPromptEvent(null);
    if (choice.outcome === "accepted") close();
  }

  return (
    <section className="card" aria-labelledby="install-card-title" style={{ borderLeft: "4px solid #3a57e8" }}>
      <h3 id="install-card-title" style={{ marginTop: 0 }}>{t("Add Waypoint to your home screen")}</h3>
      <p>{t("Open your route with one tap, full screen, even with a weak signal.")} {state === "manual" && t(manualInstallSteps(navigator.userAgent))}</p>
      <p className="muted">{t("Only today's route is kept on this device. Routes from other days are removed once everything for them has been sent.")}</p>
      <div className="row">
        {state === "prompt" && <button type="button" className="tap primary" onClick={() => void install()}>{t("Add to home screen")}</button>}
        <button type="button" className="tap" onClick={close}>{t("Not now")}</button>
      </div>
    </section>
  );
}
