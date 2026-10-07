import { useEffect, useRef } from "react";
import L from "leaflet";
import "leaflet/dist/leaflet.css";

export type LatLng = [number, number];

// Approximate depot positions (the official dataset has no coordinates); look them up with depotPosition.
export { DEPOT_LOCATIONS, depotPosition } from "../api/depots.mjs";

export type MapMarker = {
  id: string;
  at: LatLng;
  kind: "depot" | "store" | "stop" | "truck";
  color: string;
  /** Text colour of the label when it must differ from the marker colour (for contrast). */
  labelColor?: string;
  label?: string;
  title: string;
  selected?: boolean;
  onClick?: () => void;
};
export type MapLine = { id: string; points: LatLng[]; color: string; dashed?: boolean; weight?: number };

export function along(a: LatLng, b: LatLng, t: number): LatLng {
  return [a[0] + (b[0] - a[0]) * t, a[1] + (b[1] - a[1]) * t];
}

function icon(m: MapMarker) {
  const size = m.kind === "truck" ? 26 : m.kind === "depot" || m.kind === "store" ? 28 : 16;
  const glyph = m.kind === "depot" ? "D" : m.kind === "store" ? "★" : m.kind === "truck" ? "▶" : "";
  const ring = m.selected ? "box-shadow:0 0 0 8px rgba(58,87,232,.22);" : "";
  const html = `<span style="display:flex;align-items:center;justify-content:center;width:${size}px;height:${size}px;border-radius:50%;background:${m.color};border:3px solid #fff;color:#fff;font:700 ${size > 20 ? 12 : 9}px Inter,sans-serif;${ring}">${glyph}</span>`
    + (m.label ? `<span style="position:absolute;left:${size + 4}px;top:${size / 2 - 11}px;white-space:nowrap;padding:3px 7px;border-radius:4px;background:#fff;color:${m.kind === "stop" ? "#232d42" : (m.labelColor ?? m.color)};font:600 12px Inter,sans-serif;box-shadow:0 1px 4px rgba(0,0,0,.15)">${m.label.replace(/[<>&]/g, "")}</span>` : "");
  return L.divIcon({ className: "wp-map-marker", html, iconSize: [size, size], iconAnchor: [size / 2, size / 2] });
}

/**
 * OpenStreetMap map with Waypoint markers. Positions passed in may be
 * approximate; callers say so next to the map.
 */
export function WaypointMap({ markers, lines, height = 520, label, fitKey, maxZoom = 13, follow = false }: { markers: MapMarker[]; lines: MapLine[]; height?: number; label: string; fitKey?: string; maxZoom?: number; follow?: boolean }) {
  const host = useRef<HTMLDivElement>(null);
  const map = useRef<L.Map | null>(null);
  const layer = useRef<L.LayerGroup | null>(null);
  const fitted = useRef<string | undefined>(undefined);

  useEffect(() => {
    if (!host.current || map.current) return;
    map.current = L.map(host.current, { zoomControl: true, attributionControl: true }).setView([7.0, 80.2], 9);
    // OpenStreetMap blocks tile requests that carry no referrer, and the site's own policy (same-origin) sends none.
    // Only these tile images are allowed to send the site's origin (no path or query), as OSM's tile policy requires.
    L.tileLayer("https://tile.openstreetmap.org/{z}/{x}/{y}.png", { maxZoom: 18, attribution: "© OpenStreetMap contributors", referrerPolicy: "strict-origin-when-cross-origin" }).addTo(map.current);
    layer.current = L.layerGroup().addTo(map.current);
    // A map opened inside a sliding panel is measured before the panel has its final size.
    const settle = window.setTimeout(() => map.current?.invalidateSize(), 300);
    return () => { window.clearTimeout(settle); map.current?.remove(); map.current = null; };
  }, []);

  useEffect(() => {
    const m = map.current, g = layer.current;
    if (!m || !g) return;
    g.clearLayers();
    for (const line of lines) {
      if (line.points.length < 2) continue;
      L.polyline(line.points, { color: line.color, weight: line.weight ?? 4, opacity: 0.85, dashArray: line.dashed ? "4 8" : undefined }).addTo(g);
    }
    for (const marker of markers) {
      const mk = L.marker(marker.at, { icon: icon(marker), title: marker.title, keyboard: Boolean(marker.onClick), zIndexOffset: marker.kind === "truck" ? 1000 : 0 }).addTo(g);
      mk.bindTooltip(marker.title);
      if (marker.onClick) mk.on("click", marker.onClick);
    }
    const key = fitKey ?? String(markers.length);
    if (markers.length && fitted.current !== key) {
      fitted.current = key;
      m.fitBounds(L.latLngBounds(markers.map((x) => x.at)).pad(0.2), { maxZoom });
    } else if (follow && markers.length && !m.getBounds().contains(markers[0].at)) {
      // A moving truck that leaves the view is followed, without changing the zoom the person chose.
      m.panTo(markers[0].at);
    }
  }, [markers, lines, fitKey, maxZoom, follow]);

  return <div ref={host} role="region" aria-label={label} style={{ height, width: "100%", borderRadius: 8, overflow: "hidden", background: "#eef0ee" }} />;
}
