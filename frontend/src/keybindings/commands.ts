/**
 * Shortcut command catalogue (Xshell-style). Every user-facing action that
 * can carry a keybinding lives here. Ids are stable — AppConfig.shortcuts
 * persists user overrides keyed by id, and the id doubles as the i18n key
 * path (`shortcuts.cmd.<id>`).
 */

export type ShortcutCategory = "terminal" | "tab" | "pane" | "view" | "app";

export interface ShortcutCommand {
  /** Stable persisted id, e.g. "terminal.copy". */
  id: string;
  category: ShortcutCategory;
  /** Default binding in canonical form ("Ctrl+Shift+C"); null = unbound
   *  out of the box (still assignable in settings, like Xshell). */
  defaultBinding: string | null;
}

export const SHORTCUT_CATEGORIES: ShortcutCategory[] = [
  "terminal",
  "tab",
  "pane",
  "view",
  "app",
];

function cmd(
  category: ShortcutCategory,
  id: string,
  defaultBinding: string | null,
): ShortcutCommand {
  return { id, category, defaultBinding };
}

export const SHORTCUT_COMMANDS: ShortcutCommand[] = [
  // ── Terminal pane actions (applied to the focused pane of the active tab).
  // Ctrl+Shift+<key> is used instead of bare Ctrl+<key> so the control
  // sequences (Ctrl+C intr, Ctrl+F, …) keep flowing to the shell.
  cmd("terminal", "terminal.copy", "Ctrl+Shift+C"),
  cmd("terminal", "terminal.paste", "Ctrl+Shift+V"),
  cmd("terminal", "terminal.selectAll", "Ctrl+Shift+A"),
  cmd("terminal", "terminal.find", "Ctrl+Shift+F"),
  cmd("terminal", "terminal.clear", null),
  cmd("terminal", "terminal.reconnect", null),
  cmd("terminal", "terminal.zoomIn", "Ctrl+="),
  cmd("terminal", "terminal.zoomOut", "Ctrl+-"),
  cmd("terminal", "terminal.zoomReset", "Ctrl+0"),

  // ── Tab (session) actions. goto9 falls back to the LAST tab (browser
  //    convention) when fewer than nine tabs are open.
  cmd("tab", "tab.newLocal", "Ctrl+Shift+T"),
  cmd("tab", "tab.next", "Ctrl+Tab"),
  cmd("tab", "tab.prev", "Ctrl+Shift+Tab"),
  cmd("tab", "tab.close", "Ctrl+Shift+W"),
  cmd("tab", "tab.duplicate", null),
  cmd("tab", "tab.goto1", "Ctrl+1"),
  cmd("tab", "tab.goto2", "Ctrl+2"),
  cmd("tab", "tab.goto3", "Ctrl+3"),
  cmd("tab", "tab.goto4", "Ctrl+4"),
  cmd("tab", "tab.goto5", "Ctrl+5"),
  cmd("tab", "tab.goto6", "Ctrl+6"),
  cmd("tab", "tab.goto7", "Ctrl+7"),
  cmd("tab", "tab.goto8", "Ctrl+8"),
  cmd("tab", "tab.goto9", "Ctrl+9"),

  // ── Split-pane actions (max 2 panes per tab today).
  cmd("pane", "pane.splitH", null),
  cmd("pane", "pane.splitV", null),
  cmd("pane", "pane.close", null),
  cmd("pane", "pane.focusOther", null),

  // ── View toggles.
  cmd("view", "view.toggleSftp", null),
  cmd("view", "view.toggleAgent", null),
  cmd("view", "view.toggleMonitor", null),
  cmd("view", "view.toggleDocker", null),
  cmd("view", "view.toggleSidebar", null),

  // ── Application.
  cmd("app", "app.settings", "Ctrl+,"),
];

const byId = new Map(SHORTCUT_COMMANDS.map((c) => [c.id, c]));

export function commandById(id: string): ShortcutCommand | undefined {
  return byId.get(id);
}
