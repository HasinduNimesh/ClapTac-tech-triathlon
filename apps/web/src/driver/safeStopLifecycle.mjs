export function installSafeStopLock({ document, window, onLock }) {
  const onVisibilityChange = () => {
    if (document.visibilityState !== "visible") onLock(false);
  };
  const onPageHide = () => onLock(false);

  document.addEventListener("visibilitychange", onVisibilityChange);
  window.addEventListener("pagehide", onPageHide);

  return () => {
    document.removeEventListener("visibilitychange", onVisibilityChange);
    window.removeEventListener("pagehide", onPageHide);
  };
}
