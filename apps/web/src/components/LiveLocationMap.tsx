import { LiveLocation } from "../api/delivery";
import { useLocale } from "../i18n";
import { ageMinutes, isFresh } from "../dispatcher/drivenDistance.mjs";
import { MapMarker, WaypointMap } from "./WaypointMap";

/** Where the truck is, on the street map, from the driver phone's latest report. */
export function LiveLocationMap({ location, store, depot }: { location: LiveLocation | null; store?: string; depot?: string }) {
  const { t } = useLocale();
  const point = location && Number.isFinite(location.latitude) && Number.isFinite(location.longitude) &&
    typeof location.timestamp === "string" && Number.isFinite(Date.parse(location.timestamp)) ? location : null;
  const now = Date.now();
  // A parked truck reports about every 90 seconds, so only a report older than five minutes is stale.
  const stale = point ? !isFresh(point.timestamp, now) : false;
  const age = point ? ageMinutes(point.timestamp, now) : 0;
  const markers: MapMarker[] = point
    ? [{ id: "truck", at: [point.latitude, point.longitude], kind: "truck", color: stale ? "#8a92a6" : "#008b52", label: t("Truck"), title: `${t("Truck at")} ${point.latitude.toFixed(5)}, ${point.longitude.toFixed(5)}` }]
    : [];
  return <section className="card" aria-label="Live trip map">
    <h4>{t("Map")}</h4>
    {point && <WaypointMap label={t("Truck at") + " " + point.latitude.toFixed(5) + ", " + point.longitude.toFixed(5)} markers={markers} lines={[]} height={280} fitKey={point.tripId} maxZoom={15} follow />}
    {point ? <p>{t("Truck")}: {point.latitude.toFixed(5)}, {point.longitude.toFixed(5)}. {t("Last location update")}: {new Date(point.timestamp).toLocaleString()}{age >= 1 ? ` · ${age} ${t("min ago")}` : ` · ${t("just now")}`}{stale ? " (" + t("stale") + ")" : ""}</p> : <p role="status">{t("Live location unavailable.")}</p>}
    {store && <p>{t("Store")}: {store}. {t("Coordinates unavailable")}</p>}
    {depot && <p>{t("Depot")}: {depot}. {t("Coordinates unavailable")}</p>}
  </section>;
}
