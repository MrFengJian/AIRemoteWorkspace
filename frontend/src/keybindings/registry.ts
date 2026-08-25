/**
 * Imperative registries for the shortcut system: command handlers (what a
 * binding does) and per-pane terminal actions (exposed by each TerminalPanel
 * so tab-level code can drive copy/paste/find/zoom on the focused pane).
 */

export type ShortcutHandler = () => void;

const handlers = new Map<string, ShortcutHandler>();

/** Register the handler for a command id. Returns an unregister function. */
export function registerShortcutHandler(
  id: string,
  handler: ShortcutHandler,
): () => void {
  handlers.set(id, handler);
  return () => {
    if (handlers.get(id) === handler) handlers.delete(id);
  };
}

export function getShortcutHandler(id: string): ShortcutHandler | undefined {
  return handlers.get(id);
}

/** Terminal actions exposed by one mounted pane (xterm instance). */
export interface PaneActions {
  copy: () => void;
  paste: () => void;
  selectAll: () => void;
  /** Open the in-buffer search bar. */
  find: () => void;
  clear: () => void;
  zoomIn: () => void;
  zoomOut: () => void;
  zoomReset: () => void;
  focus: () => void;
}

const paneActions = new Map<string, PaneActions>();

/** Register the actions of a mounted pane (keyed by its backend session id). */
export function registerPaneActions(
  paneId: string,
  actions: PaneActions,
): () => void {
  paneActions.set(paneId, actions);
  return () => {
    if (paneActions.get(paneId) === actions) paneActions.delete(paneId);
  };
}

export function getPaneActions(paneId: string): PaneActions | undefined {
  return paneActions.get(paneId);
}

/**
 * Exclusive keyboard-capture guard. The settings page's shortcut recorder
 * sets this while listening for a new combination so the global dispatcher
 * (registered earlier on the same node) lets the keys through to it alone.
 */
let keyCaptured = false;

export function setKeyCapture(active: boolean) {
  keyCaptured = active;
}

export function isKeyCaptured(): boolean {
  return keyCaptured;
}
