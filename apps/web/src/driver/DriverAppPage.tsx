import { useLocale } from "../i18n";
import { useDriverApk } from "../routes/useDriverApk";

/**
 * Drivers work in the Android app, so the driver screens of the web app are switched off: /driver and
 * /driver/trips both land here, with the download. (The old pages stay in the source for their tests.)
 */
export function DriverAppPage() {
  const { t } = useLocale();
  const apk = useDriverApk();
  return (
    <section className="card" aria-labelledby="driver-app-title">
      <h1 id="driver-app-title" style={{ marginTop: 0 }}>{t("Drivers use the Waypoint Driver app")}</h1>
      <p>{t("Your route, stops, proof of delivery and offline sync are in the mobile app. The web version of the driver screens is turned off.")}</p>
      {apk === undefined ? (
        <p role="status">{t("Checking for the latest app…")}</p>
      ) : apk ? (
        <>
          <a className="tap primary" href="/downloads/waypoint-driver.apk" download>{t("Download the Android app")}</a>
          <p className="muted">
            {t("Version")} <b>{apk.version}</b> · <b>{apk.sizeLabel}</b>
            {apk.builtOn ? <> · {t("Updated")} <b>{apk.builtOn}</b></> : null} · {t("Android 8.0 or newer")}
          </p>
          <h2>{t("How to install")}</h2>
          <ol>
            <li>{t("Download the file to your phone.")}</li>
            <li>{t("Open it, and allow installs from this source if Android asks.")}</li>
            <li>{t("Sign in with the account your depot supervisor created.")}</li>
          </ol>
          <p className="muted">{t("This app is not on Google Play. Install it only from this page. SHA-256")}: <code>{apk.sha256}</code></p>
        </>
      ) : (
        <p role="status">{t("The Android app has not been published yet. Ask your depot supervisor.")}</p>
      )}
    </section>
  );
}
