/** What the bell shows: which items are unread, the badge text and the accessible name. */

/** Items that still need the person. Only items flagged `readable` can be dismissed locally; the rest clear when the server state changes. */
export function unreadItems(items, readKeys) {
  const read = readKeys instanceof Set ? readKeys : new Set(readKeys || []);
  return (items || []).filter((item) => !(item.readable && read.has(item.key)));
}

export function countUnread(items, readKeys) {
  return unreadItems(items, readKeys).length;
}

/** Empty for zero, the number up to 99, then "99+". */
export function badgeText(count) {
  const n = Number.isFinite(count) ? Math.max(0, Math.floor(count)) : 0;
  if (n === 0) return "";
  return n > 99 ? "99+" : String(n);
}

/** "Notifications, 3 unread". The count is spoken in full (not "99+") so it is never misleading. */
export function bellLabel(count, t = (s) => s) {
  const n = Number.isFinite(count) ? Math.max(0, Math.floor(count)) : 0;
  return `${t("Notifications")}, ${n} ${t("unread")}`;
}
