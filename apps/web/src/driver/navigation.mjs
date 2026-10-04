// Where a driver is sent to reach a stop. The same rule the phone app follows: navigate to an exact,
// recorded position; never to an approximate one (a district centre is not the shop), so then open a
// search for the shop by name and say so.

/**
 * Returns { href, exact }. exact is true when href navigates to the recorded position.
 */
export function navigationTarget(stop) {
  const lat = stop?.latitude;
  const lng = stop?.longitude;
  if (typeof lat === "number" && typeof lng === "number" && Number.isFinite(lat) && Number.isFinite(lng) && stop.locationApproximate !== true) {
    return { exact: true, href: `https://www.google.com/maps/dir/?api=1&destination=${lat},${lng}&travelmode=driving` };
  }
  const query = [stop?.outletName || stop?.outletId, stop?.district, "Sri Lanka"].filter(Boolean).join(", ");
  return { exact: false, href: `https://www.google.com/maps/search/?api=1&query=${encodeURIComponent(query)}` };
}
