export type NotificationItem = {
  id: string;
  type: string;
  title: string;
  body: string;
  resourceType?: string;
  resourceId?: string;
  readAt?: string;
  createdAt: string;
};

/**
 * notificationTarget is where clicking a notification goes.
 *
 * It reads the same two fields the notification email builds its link from, so
 * one opened in the app and one opened from an email land in the same place. A
 * notification about something with no screen of its own goes nowhere rather
 * than to a page that cannot show it.
 */
export function notificationTarget(
  resourceType: string | undefined,
  resourceId: string | undefined,
): string {
  if (!resourceId) return "";
  switch ((resourceType ?? "").toUpperCase()) {
    case "DOCUMENT":
      return `/docs/${resourceId}`;
    case "WORKSPACE":
      return `/workspace/${resourceId}`;
    default:
      return "";
  }
}

/** unreadLabel is what the badge shows; anything past 99 is just "many". */
export function unreadLabel(count: number): string {
  if (count <= 0) return "";
  return count > 99 ? "99+" : String(count);
}
