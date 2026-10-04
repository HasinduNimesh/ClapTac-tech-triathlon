import { useLocale } from "../i18n";
import { conflictMessage, syncStateLabel } from "../offline/syncQueue.mjs";
import type { StopSyncState, SyncConflictNotice } from "../offline/syncQueue.mjs";

type Props = {
  status: { state: StopSyncState; waiting: number; failedItems: { operationId: string; reason: string }[] };
  notices: SyncConflictNotice[];
  onRetry: (operationId: string) => void;
  onDismiss: (operationId: string) => void;
};

// Existing text classes: muted for saved, blue for sending, green for sent, red for failed.
const CHIP_TONE: Record<StopSyncState, string> = { none: "muted", saved: "muted", sending: "status-syncing", sent: "status-ok", failed: "status-bad" };

/** Per-stop sync state (Saved on this phone / Sending / Sent), Retry for a failed send, and plan-version notices. */
export function StopSyncStatus({ status, notices, onRetry, onDismiss }: Props) {
  const { t } = useLocale();
  if (status.state === "none" && notices.length === 0) return null;
  const waiting = status.state === "saved" || status.state === "sending" || status.state === "failed";
  return (
    <div className="sync-status">
      {status.state !== "none" && (
        <p className={`sync-chip ${CHIP_TONE[status.state]}`} role="status">
          {t(syncStateLabel(status.state))}{waiting && status.waiting > 0 ? ` · ${t(`${status.waiting} waiting`)}` : ""}
        </p>
      )}
      {status.failedItems.length > 0 && (
        <div className="sync-retry">
          <p className="status-bad" role="alert">{t("This could not be sent. Your record is still saved on this phone.")}</p>
          <button type="button" className="tap" onClick={() => onRetry(status.failedItems[0].operationId)}>{t("Retry")}</button>
        </div>
      )}
      {notices.map((notice) => (
        <div key={notice.operationId} className="sync-conflict" role="status">
          <p>{conflictMessage(notice, t)}</p>
          <button type="button" className="tap" onClick={() => onDismiss(notice.operationId)}>{t("Got it")}</button>
        </div>
      ))}
    </div>
  );
}
