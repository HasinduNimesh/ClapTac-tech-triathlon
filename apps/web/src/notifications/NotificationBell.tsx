import { useEffect, useId, useRef, useState } from "react";
import { Link } from "react-router-dom";
import { useLocale } from "../i18n";
import { badgeText, bellLabel, type BellItem } from "./bellModel.mjs";
import type { NotificationFeed } from "./useRoleNotifications";
import "./notificationBell.css";

const PREVIEW_LIMIT = 6;

/**
 * XR-3: the header bell shared by the role workspaces. The badge counts what still needs the person
 * (the same items as the role's notifications page); the panel previews the newest few and links to
 * the full page. Critical items (breakdowns, temperature) also reach stores by text message from the
 * server, so the bell never has to be open for those to be seen.
 */
export function NotificationBell({ feed, allTo, icon, className = "dp-bell" }: { feed: NotificationFeed; allTo: string; icon: string; className?: string }) {
  const { t } = useLocale();
  const [open, setOpen] = useState(false);
  const wrap = useRef<HTMLDivElement>(null);
  const panelId = useId();
  const unread = feed.unreadCount;
  const visible = feed.unread.slice(0, PREVIEW_LIMIT);

  useEffect(() => {
    if (!open) return;
    const onKey = (event: KeyboardEvent) => { if (event.key === "Escape") setOpen(false); };
    const onClick = (event: MouseEvent) => { if (wrap.current && !wrap.current.contains(event.target as Node)) setOpen(false); };
    document.addEventListener("keydown", onKey);
    document.addEventListener("mousedown", onClick);
    return () => { document.removeEventListener("keydown", onKey); document.removeEventListener("mousedown", onClick); };
  }, [open]);

  const itemLink = (item: BellItem, body: JSX.Element) => {
    const close = () => { setOpen(false); if (item.readable) feed.markRead([item.key]); };
    if (item.to) return <Link to={item.to} className="nb-item-link" onClick={close}>{body}</Link>;
    if (item.href) return <a href={item.href} className="nb-item-link" onClick={close}>{body}</a>;
    return <Link to={allTo} className="nb-item-link" onClick={close}>{body}</Link>;
  };

  return (
    <div className="nb-wrap" ref={wrap}>
      <button type="button" className={`${className} nb-trigger`} aria-label={bellLabel(unread, t)} aria-expanded={open} aria-controls={panelId} onClick={() => setOpen((v) => !v)}>
        <img src={icon} alt="" width={16} height={17} />
        {unread > 0 && <span className="dp-bell-dot" aria-hidden="true">{badgeText(unread)}</span>}
      </button>
      {open && (
        <div id={panelId} className="nb-panel" role="dialog" aria-label={t("Notifications")}>
          <div className="nb-head">
            <strong>{t("Notifications")}</strong>
            {feed.items.some((item) => item.readable) && <button type="button" className="nb-link" onClick={feed.markAllRead}>{t("Mark all read")}</button>}
          </div>
          {feed.loading && feed.items.length === 0 && <p className="nb-empty" role="status">{t("Loading…")}</p>}
          {feed.error && <p className="nb-error">{t("Some notifications could not be loaded.")}</p>}
          {!feed.loading && visible.length === 0 && <p className="nb-empty">{t("Nothing needs you right now.")}</p>}
          <ul className="nb-list">
            {visible.map((item) => (
              <li key={item.key} className={`nb-item nb-item--${item.tone || "muted"}`}>
                {itemLink(item, <>
                  {item.tag && <span className="nb-tag">{item.tag}</span>}
                  <span className="nb-title">{item.title}</span>
                  {item.text && <span className="nb-text">{item.text}</span>}
                </>)}
              </li>
            ))}
          </ul>
          <Link to={allTo} className="nb-all" onClick={() => setOpen(false)}>{feed.unread.length > visible.length ? `${t("See all")} (${feed.unread.length})` : t("Open notifications")}</Link>
        </div>
      )}
    </div>
  );
}
