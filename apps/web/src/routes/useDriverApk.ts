import { useEffect, useState } from "react";
import { describeApk, type ApkInfo } from "./apkInfo.mjs";

/** The published Android driver app, read from /downloads/waypoint-driver.json. undefined while checking, null when there is none. */
export function useDriverApk(): ApkInfo | null | undefined {
  const [apk, setApk] = useState<ApkInfo | null | undefined>(undefined);
  useEffect(() => {
    let active = true;
    fetch("/downloads/waypoint-driver.json", { cache: "no-store" })
      .then((r) => (r.ok ? r.json() : null))
      .then((meta) => { if (active) setApk(describeApk(meta)); })
      .catch(() => { if (active) setApk(null); });
    return () => { active = false; };
  }, []);
  return apk;
}
