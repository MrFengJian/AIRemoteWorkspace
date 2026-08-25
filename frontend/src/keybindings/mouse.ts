/**
 * Mouse behaviour configuration (Xshell-style). These live alongside the
 * keyboard shortcuts in Settings → Shortcuts and in the keybinding store,
 * but they are select-from-options settings rather than recordable combos.
 */

/** Middle-click actions available on terminal panes. */
export const MIDDLE_CLICK_ACTIONS = [
  "none",
  "pasteSelection",
  "pasteClipboard",
  "sendEnter",
  "contextMenu",
] as const;

export type MiddleClickAction = (typeof MIDDLE_CLICK_ACTIONS)[number];

/** Xshell default: middle-click pastes the current selection. */
export const DEFAULT_MIDDLE_CLICK_ACTION: MiddleClickAction = "pasteSelection";

/** Coerce a persisted/unknown value into a valid action. */
export function normalizeMiddleClickAction(
  value: string | null | undefined,
): MiddleClickAction {
  return MIDDLE_CLICK_ACTIONS.find((a) => a === value) ?? DEFAULT_MIDDLE_CLICK_ACTION;
}
