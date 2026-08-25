import { useEffect } from "react";

import { ConfigService } from "@/../bindings/github.com/ai-remote/workspace/internal/interfaces";
import { describeEvent } from "@/keybindings/match";
import {
  getShortcutHandler,
  isKeyCaptured,
  registerShortcutHandler,
  type ShortcutHandler,
} from "@/keybindings/registry";
import { useKeybindingStore } from "@/keybindings/store";

/**
 * Global shortcut dispatcher — mounted once in AppShell. A single
 * window-level keydown listener in the CAPTURE phase: it fires before
 * xterm's textarea handlers, so a matched binding is swallowed
 * (preventDefault + stopPropagation) and never reaches the PTY; unmatched
 * keys keep flowing to whatever is focused. Also loads the persisted
 * overrides once at startup.
 */
export function useShortcutDispatcher() {
  useEffect(() => {
    ConfigService.GetAppConfig()
      .then((cfg) => useKeybindingStore.getState().load(cfg.shortcuts ?? {}))
      .catch(() => {});

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
    return () => window.removeEventListener("keydown", onKeyDown, true);
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
