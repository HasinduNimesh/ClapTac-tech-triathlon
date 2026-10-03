import { useRef } from "react";
import { LiveLocation } from "../api/delivery";
import { useLocale } from "../i18n";

export function LiveLocationMap({ location, store, depot }: { location: LiveLocation | null; store?: string; depot?: string }) {
  const { t } = useLocale();
  const point = location && Number.isFinite(location.latitude) && Number.isFinite(location.longitude) &&
    typeof location.timestamp === "string" && Number.isFinite(Date.parse(location.timestamp)) ? location : null;
  const stale = point ? Date.now() - Date.parse(point.timestamp) > 60_000 : false;
  const origin = useRef<{ latitude: number; longitude: number } | null>(null);
  if (point && !origin.current) origin.current = { latitude: point.latitude, longitude: point.longitude };
  const x = point && origin.current ? Math.max(16, Math.min(284, 150 + (point.longitude - origin.current.longitude) * 6000)) : 150;
  const y = point && origin.current ? Math.max(16, Math.min(164, 90 - (point.latitude - origin.current.latitude) * 6000)) : 90;
  return <section className="card" aria-label="Live trip map">
    <h4>{t("Map")}</h4>
    <svg viewBox="0 0 300 180" role="img" aria-label={point ? t("Truck at") + " " + point.latitude.toFixed(5) + ", " + point.longitude.toFixed(5) : t("Truck position unavailable")} style={{ width: "100%", maxWidth: 500, background: "#e8f1f3", border: "1px solid #9db9bd" }}>
      <path d="M0 45H300 M0 90H300 M0 135H300 M75 0V180 M150 0V180 M225 0V180" stroke="#b5cdd0" fill="none" />
      {point && <><circle cx={x} cy={y} r="10" fill="#145a74" stroke="white" strokeWidth="3" /><text x={Math.min(x + 14, 245)} y={Math.max(y - 12, 20)} fill="#12313d" fontSize="13">{t("Truck")}</text></>}
    </svg>
    {point ? <p>{t("Truck")}: {point.latitude.toFixed(5)}, {point.longitude.toFixed(5)}. {t("Last location update")}: {new Date(point.timestamp).toLocaleString()}{stale ? " (" + t("stale") + ")" : ""}</p> : <p role="status">{t("Live location unavailable.")}</p>}
    {store && <p>{t("Store")}: {store}. {t("Coordinates unavailable")}</p>}
    {depot && <p>{t("Depot")}: {depot}. {t("Coordinates unavailable")}</p>}
  </section>;
}
