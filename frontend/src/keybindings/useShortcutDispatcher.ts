import { useEffect } from "react";
import { Events } from "@wailsio/runtime";

import { ConfigService } from "@/../bindings/github.com/ai-remote/workspace/internal/interfaces";
import { describeEvent } from "@/keybindings/match";
import { normalizeMiddleClickAction } from "@/keybindings/mouse";
import {
  getShortcutHandler,
  isKeyCaptured,
  registerShortcutHandler,
  type ShortcutHandler,
} from "@/keybindings/registry";
import { useKeybindingStore } from "@/keybindings/store";

/**
 * Global shortcut dispatcher — mounted once in AppShell (and in the
 * standalone SFTP window). A single window-level keydown listener in the
 * CAPTURE phase: it fires before xterm's textarea handlers, so a matched
 * binding is swallowed (preventDefault + stopPropagation) and never reaches
 * the PTY; unmatched keys keep flowing to whatever is focused. Also loads
 * the persisted overrides at startup and re-loads them when any window
 * saves new settings ("config:changed" broadcast).
 */
export function useShortcutDispatcher() {
  useEffect(() => {
    const load = () => {
      ConfigService.GetAppConfig()
        .then((cfg) => {
          const state = useKeybindingStore.getState();
          state.load(cfg.shortcuts ?? {});
          state.setMouseMiddleClick(normalizeMiddleClickAction(cfg.middleClickAction));
        })
        .catch(() => {});
    };
    load();
    const offConfigChanged = Events.On("config:changed", load);

    const onKeyDown = (e: KeyboardEvent) => {
      if (e.isComposing || e.repeat || isKeyCaptured()) return;
      const binding = describeEvent(e);
      if (!binding) return;
      const id = useKeybindingStore.getState().byBinding.get(binding);
      if (!id) return;
      const handler = getShortcutHandler(id);
      if (!handler) return;
      e.preventDefault();
      e.stopPropagation();
      handler();
    };
    window.addEventListener("keydown", onKeyDown, true);
    return () => {
      window.removeEventListener("keydown", onKeyDown, true);
      offConfigChanged();
    };
  }, []);
}

/**
 * Register a map of command handlers for the lifetime of the calling
 * component. Re-registers on every render (no dep array) so the handler
 * closures always see fresh state — registration is a Map set, cheap
 * enough to redo per render.
 */
export function useShortcutHandlers(map: Record<string, ShortcutHandler>) {
  useEffect(() => {
    const unregisters = Object.entries(map).map(([id, fn]) =>
      registerShortcutHandler(id, fn),
    );
    return () => unregisters.forEach((un) => un());
  });
}
