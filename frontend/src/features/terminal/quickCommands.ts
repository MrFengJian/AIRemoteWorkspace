import { useCallback, useEffect, useState } from "react";
import { Events } from "@wailsio/runtime";

import { ConfigService } from "@/../bindings/github.com/ai-remote/workspace/internal/interfaces";
import type { QuickCommand } from "@/../bindings/github.com/ai-remote/workspace/internal/domain";

export type { QuickCommand };

/**
 * useQuickCommands subscribes to the user's quick command bar entries
 * (AppConfig.quickCommands). Loads on mount and re-reads whenever a config
 * save fires "config:changed" — the same channel the highlight rules ride,
 * so the bar and the manage dialog (possibly saving each other) stay in sync.
 */
export function useQuickCommands(): QuickCommand[] {
  const [commands, setCommands] = useState<QuickCommand[]>([]);

  const reload = useCallback(() => {
    ConfigService.GetAppConfig()
      .then((cfg) => setCommands(cfg.quickCommands ?? []))
      .catch(() => {});
  }, []);

  useEffect(() => {
    reload();
    const cancel = Events.On("config:changed", reload);
    return () => {
      if (typeof cancel === "function") cancel();
    };
  }, [reload]);

  return commands;
}

/**
 * Persist the quick command list. Read-modify-write against a FRESH config
 * so fields edited elsewhere (settings view, appearance dialog) are never
 * clobbered by a stale local copy, then broadcast "config:changed" so every
 * subscriber re-reads.
 */
export async function saveQuickCommands(next: QuickCommand[]): Promise<void> {
  const cfg = await ConfigService.GetAppConfig();
  await ConfigService.SetAppConfig({ ...cfg, quickCommands: next });
  void Events.Emit("config:changed");
}

/** localStorage flag that disables the multi-target send confirmation. */
export const QUICKCMD_SKIP_CONFIRM_KEY = "quickcmd-skip-confirm";

/**
 * Turn a quick command into raw PTY input: line breaks become carriage
 * returns (a shell needs \r to accept a line) and, when the entry says so, a
 * trailing \r executes the last line on arrival. Without it the payload is
 * only typed — the user reviews and presses Enter themselves.
 */
export function quickCommandPayload(cmd: QuickCommand): string {
  const body = cmd.command.replace(/\r?\n/g, "\r");
  return cmd.sendEnter ? `${body}\r` : body;
}
