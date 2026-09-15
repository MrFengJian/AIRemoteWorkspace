/**
 * systemNotify fires an OS-level notification via the Web Notification API
 * (WebView2/WebView surfaces it as a native toast). Best-effort by design:
 * unsupported platforms, denied permission, or host-side failures degrade to
 * a silent no-op — notifications are a convenience, never a dependency.
 */
export async function systemNotify(title: string, body: string): Promise<void> {
  try {
    if (typeof Notification === "undefined") return;
    if (Notification.permission === "denied") return;
    if (Notification.permission === "default") {
      const granted = await Notification.requestPermission();
      if (granted !== "granted") return;
    }
    new Notification(title, { body, silent: true });
  } catch {
    /* best-effort */
  }
}

/** True when the app window is NOT the focused window (notifications are
 * only worth firing when the user is looking elsewhere). */
export function windowUnfocused(): boolean {
  return typeof document === "undefined" ? false : !document.hasFocus();
}
