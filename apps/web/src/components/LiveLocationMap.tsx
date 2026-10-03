import { useRef } from "react";
import { LiveLocation } from "../api/delivery";

export function LiveLocationMap({ location, store, depot }: { location: LiveLocation | null; store?: string; depot?: string }) {
  const origin = useRef<{ latitude: number; longitude: number } | null>(null);
  if (location && !origin.current) origin.current = { latitude: location.latitude, longitude: location.longitude };
  const x = location && origin.current ? Math.max(16, Math.min(284, 150 + (location.longitude - origin.current.longitude) * 6000)) : 150;
  const y = location && origin.current ? Math.max(16, Math.min(164, 90 - (location.latitude - origin.current.latitude) * 6000)) : 90;
  return <section className="card" aria-label="Live trip map">
    <h4>Map</h4>
    <svg viewBox="0 0 300 180" role="img" aria-label={location ? "Truck at " + location.latitude.toFixed(5) + ", " + location.longitude.toFixed(5) : "Truck position unavailable"} style={{ width: "100%", maxWidth: 500, background: "#e8f1f3", border: "1px solid #9db9bd" }}>
      <path d="M0 45H300 M0 90H300 M0 135H300 M75 0V180 M150 0V180 M225 0V180" stroke="#b5cdd0" fill="none" />
      {location && <><circle cx={x} cy={y} r="10" fill="#145a74" stroke="white" strokeWidth="3" /><text x={Math.min(x + 14, 245)} y={Math.max(y - 12, 20)} fill="#12313d" fontSize="13">Truck</text></>}
    </svg>
    {location ? <p>Truck ? {location.latitude.toFixed(5)}, {location.longitude.toFixed(5)} ? Last location update: {new Date(location.timestamp).toLocaleString()}</p> : <p role="status">Live location unavailable.</p>}
    {store && <p>Store: {store} ? Coordinates unavailable</p>}
    {depot && <p>Depot: {depot} ? Coordinates unavailable</p>}
  </section>;
}
