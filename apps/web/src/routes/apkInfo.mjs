// What the home page says about the Android app file. The build script writes
// /downloads/waypoint-driver.json next to the APK; if it is missing or malformed the page says the app
// is not published instead of linking to a file that may not be there.

export function formatBytes(bytes) {
  if (!Number.isFinite(bytes) || bytes <= 0) return "";
  const units = ["B", "KB", "MB", "GB"];
  let value = bytes;
  let unit = 0;
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024;
    unit += 1;
  }
  return `${unit === 0 ? value : value.toFixed(1)} ${units[unit]}`;
}

/** Returns null unless the metadata describes a downloadable file. */
export function describeApk(meta) {
  if (!meta || typeof meta !== "object") return null;
  const version = typeof meta.version === "string" ? meta.version.trim() : "";
  const sha256 = typeof meta.sha256 === "string" ? meta.sha256.trim().toLowerCase() : "";
  const size = Number(meta.size);
  if (!version || !/^[0-9a-f]{64}$/.test(sha256) || !Number.isInteger(size) || size <= 0) return null;
  const built = typeof meta.builtAt === "string" ? new Date(meta.builtAt) : undefined;
  const builtOn = built && !Number.isNaN(built.getTime())
    ? new Intl.DateTimeFormat("en-GB", { day: "numeric", month: "short", year: "numeric", timeZone: "Asia/Colombo" }).format(built)
    : "";
  return { version, sizeLabel: formatBytes(size), sha256, shaShort: `${sha256.slice(0, 8)}…${sha256.slice(-8)}`, builtOn };
}
