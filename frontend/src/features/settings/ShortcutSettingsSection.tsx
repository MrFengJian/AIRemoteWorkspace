import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Keyboard, Mouse, RotateCcw } from "lucide-react";

import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Select } from "@/components/ui/select";
import type { AppConfig } from "@/../bindings/github.com/ai-remote/workspace/internal/domain/models";
import {
  SHORTCUT_CATEGORIES,
  SHORTCUT_COMMANDS,
  type ShortcutCommand,
} from "@/keybindings/commands";
import { describeEvent, isBindingAllowed } from "@/keybindings/match";
import {
  MIDDLE_CLICK_ACTIONS,
  normalizeMiddleClickAction,
  type MiddleClickAction,
} from "@/keybindings/mouse";
import { setKeyCapture } from "@/keybindings/registry";
import { useKeybindingStore } from "@/keybindings/store";
import { cn } from "@/lib/utils";

/**
 * Settings → Shortcuts (Xshell-style keyboard page): every command grouped
 * by category, click a binding to record a new combination. Validation
 * blocks dangerous bare-letter bindings; conflicts with other commands are
 * rejected with a message naming the conflicting command.
 */

/** The Wails-generated AppConfig type makes map values optional; drop the
 *  undefined entries so the rest of the file works with plain strings. */
function cleanOverrides(
  raw: { [_ in string]?: string } | null | undefined,
): Record<string, string> {
  const out: Record<string, string> = {};
  for (const [k, v] of Object.entries(raw ?? {})) {
    if (typeof v === "string") out[k] = v;
  }
  return out;
}

export function ShortcutSettingsSection({
  config,
  update,
}: {
  config: AppConfig;
  update: (patch: Partial<AppConfig>) => Promise<boolean>;
}) {
  const { t } = useTranslation();
  const resolved = useKeybindingStore((s) => s.resolved);
  const [recordingId, setRecordingId] = useState<string | null>(null);
  const [error, setError] = useState<{ id: string; message: string } | null>(null);

  const overrides = cleanOverrides(config.shortcuts);

  /** Persist a new overrides map and apply it to the runtime store. On a
   *  failed save the store is rolled back so bindings match what's stored. */
  const applyOverrides = async (next: Record<string, string>) => {
    const prev = overrides;
    useKeybindingStore.getState().load(next);
    const ok = await update({ shortcuts: next });
    if (!ok) useKeybindingStore.getState().load(prev);
  };

  const resetCommand = (id: string) => {
    const next = { ...overrides };
    delete next[id];
    void applyOverrides(next);
  };

  const resetAll = () => {
    void applyOverrides({});
  };

  /** Persist a new middle-click action and apply it to the runtime store
   *  immediately; roll the store back if the save fails. */
  const applyMiddleClick = async (action: MiddleClickAction) => {
    const prev = normalizeMiddleClickAction(config.middleClickAction);
    useKeybindingStore.getState().setMouseMiddleClick(action);
    const ok = await update({ middleClickAction: action });
    if (!ok) useKeybindingStore.getState().setMouseMiddleClick(prev);
  };

  /** Name of the (other) command already using this binding, if any. */
  const conflictWith = (binding: string, selfId: string): string | null => {
    for (const other of SHORTCUT_COMMANDS) {
      if (other.id !== selfId && resolved[other.id] === binding) {
        return t(`shortcuts.cmd.${other.id}`);
      }
    }
    return null;
  };

  // Key recorder: one window-level capture listener while a row is armed.
  // setKeyCapture makes the global dispatcher ignore keys so the pressed
  // combination reaches the recorder alone.
  useEffect(() => {
    if (!recordingId) return;
    setKeyCapture(true);
    const onKey = (e: KeyboardEvent) => {
      e.preventDefault();
      e.stopPropagation();
      e.stopImmediatePropagation();
      if (e.key === "Escape") {
        setRecordingId(null);
        setError(null);
        return;
      }
      const bare = !e.ctrlKey && !e.altKey && !e.metaKey && !e.shiftKey;
      if (e.key === "Backspace" && bare) {
        // Clear the binding entirely (the command keeps existing, just
        // unbound — stored as the "" override).
        setRecordingId(null);
        setError(null);
        void applyOverrides({ ...overrides, [recordingId]: "" });
        return;
      }
      const binding = describeEvent(e);
      if (!binding) return; // pure modifier press — keep listening
      if (!isBindingAllowed(binding)) {
        setError({ id: recordingId, message: t("shortcuts.invalid") });
        return;
      }
      const other = conflictWith(binding, recordingId);
      if (other) {
        setError({
          id: recordingId,
          message: t("shortcuts.conflictWith", { command: other }),
        });
        return;
      }
      setRecordingId(null);
      setError(null);
      void applyOverrides({ ...overrides, [recordingId]: binding });
    };
    window.addEventListener("keydown", onKey, true);
    return () => {
      setKeyCapture(false);
      window.removeEventListener("keydown", onKey, true);
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [recordingId]);

  const row = (command: ShortcutCommand) => {
    const overridden = command.id in overrides;
    return (
      <div key={command.id} className="flex items-center justify-between gap-3 py-1">
        <span className="min-w-0 truncate text-sm">
          {t(`shortcuts.cmd.${command.id}`)}
        </span>
        <div className="flex shrink-0 items-center gap-1.5">
          {error?.id === command.id && (
            <span className="max-w-56 truncate text-xs text-destructive" title={error.message}>
              {error.message}
            </span>
          )}
          <button
            type="button"
            onClick={() => {
              setError(null);
              setRecordingId(recordingId === command.id ? null : command.id);
            }}
            className={cn(
              "inline-flex h-7 min-w-28 items-center justify-center rounded-[var(--radius)] border px-2.5 font-mono text-xs transition-colors",
              recordingId === command.id
                ? "animate-pulse border-primary text-primary"
                : "border-input bg-background text-foreground hover:bg-accent",
            )}
          >
            {recordingId === command.id
              ? t("shortcuts.recording")
              : resolved[command.id] ?? t("shortcuts.unbound")}
          </button>
          {overridden && (
            <button
              type="button"
              aria-label={t("shortcuts.reset")}
              title={t("shortcuts.reset")}
              onClick={() => resetCommand(command.id)}
              className="rounded p-1.5 text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
            >
              <RotateCcw className="h-3.5 w-3.5" />
            </button>
          )}
        </div>
      </div>
    );
  };

  const modifiedCount = Object.keys(overrides).length;

  return (
    <div className="flex flex-col gap-4">
      <h2 className="text-lg font-semibold">{t("settings.shortcuts")}</h2>
      <Card>
        <CardHeader className="flex flex-row items-start justify-between space-y-0">
          <div className="space-y-1.5">
            <CardTitle className="flex items-center gap-2 text-sm">
              <Keyboard className="h-4 w-4" /> {t("shortcuts.title")}
            </CardTitle>
            <CardDescription>{t("shortcuts.desc")}</CardDescription>
          </div>
          <button
            type="button"
            disabled={modifiedCount === 0}
            onClick={resetAll}
            className="inline-flex h-8 shrink-0 items-center gap-1.5 rounded-[var(--radius)] border border-input bg-background px-3 text-xs font-medium transition-colors hover:bg-accent disabled:pointer-events-none disabled:opacity-40"
          >
            <RotateCcw className="h-3.5 w-3.5" /> {t("shortcuts.resetAll")}
          </button>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          {/* Mouse behaviours — select-style options, not recordable combos. */}
          <div className="flex flex-col">
            <h3 className="mb-1 flex items-center gap-1.5 border-b border-border pb-1 text-xs font-semibold uppercase tracking-wider text-muted-foreground">
              <Mouse className="h-3.5 w-3.5" /> {t("shortcuts.mouse")}
            </h3>
            <div className="flex items-center justify-between gap-3 py-1">
              <span className="min-w-0 truncate text-sm">{t("shortcuts.middleClick")}</span>
              <Select
                value={normalizeMiddleClickAction(config.middleClickAction)}
                onChange={(e) => void applyMiddleClick(e.target.value as MiddleClickAction)}
                className="w-56"
              >
                {MIDDLE_CLICK_ACTIONS.map((action) => (
                  <option key={action} value={action}>
                    {t(`shortcuts.middleClickAction.${action}`)}
                  </option>
                ))}
              </Select>
            </div>
          </div>

          {SHORTCUT_CATEGORIES.map((category) => (
            <div key={category} className="flex flex-col">
              <h3 className="mb-1 border-b border-border pb-1 text-xs font-semibold uppercase tracking-wider text-muted-foreground">
                {t(`shortcuts.cat.${category}`)}
              </h3>
              {SHORTCUT_COMMANDS.filter((c) => c.category === category).map(row)}
            </div>
          ))}
        </CardContent>
      </Card>
    </div>
  );
}
