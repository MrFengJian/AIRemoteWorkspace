import { create } from "zustand";

import { SHORTCUT_COMMANDS, commandById } from "@/keybindings/commands";
import { formatBinding, parseBinding } from "@/keybindings/match";

/**
 * Sanitize persisted overrides: drop unknown command ids (from an older or
 * newer build), undefined values (Wails generates the map as optional) and
 * unparsable strings; canonicalize the rest. "" means the user explicitly
 * unbound a default binding.
 */
function sanitize(
  raw: Record<string, string | undefined> | null | undefined,
): Record<string, string> {
  const out: Record<string, string> = {};
  if (!raw) return out;
  for (const [id, value] of Object.entries(raw)) {
    if (!commandById(id)) continue;
    if (value === "") {
      out[id] = "";
      continue;
    }
    if (typeof value !== "string") continue;
    const parsed = parseBinding(value);
    if (parsed) out[id] = formatBinding(parsed);
  }
  return out;
}

interface KeybindingState {
  /** User overrides only: id → canonical binding ("" = explicitly unbound). */
  overrides: Record<string, string>;
  /** Override ?? default per command; null = currently unbound. */
  resolved: Record<string, string | null>;
  /** Canonical binding → command id, for the dispatcher. */
  byBinding: Map<string, string>;

  /** Load persisted overrides (startup + settings saves). */
  load: (raw: Record<string, string | undefined> | null | undefined) => void;
  /** Apply one override (binding) or remove it (null = back to default). */
  setOverride: (id: string, binding: string | null) => void;
  /** Drop a single override (command returns to its default binding). */
  resetCommand: (id: string) => void;
  resetAll: () => void;
}

function recompute(overrides: Record<string, string>) {
  const resolved: Record<string, string | null> = {};
  const byBinding = new Map<string, string>();
  for (const command of SHORTCUT_COMMANDS) {
    const override = overrides[command.id];
    const binding =
      override !== undefined ? override || null : command.defaultBinding;
    resolved[command.id] = binding;
    // First command wins on a (conflicting) duplicate binding — the settings
    // UI blocks conflicts, this is just a defensive fallback.
    if (binding && !byBinding.has(binding)) byBinding.set(binding, command.id);
  }
  return { resolved, byBinding };
}

export const useKeybindingStore = create<KeybindingState>((set) => ({
  overrides: {},
  ...recompute({}),

  load: (raw) =>
    set((s) => {
      const overrides = sanitize(raw);
      if (
        Object.keys(overrides).length === Object.keys(s.overrides).length &&
        Object.entries(overrides).every(([k, v]) => s.overrides[k] === v)
      ) {
        return {}; // no change — keep object identities stable
      }
      return { overrides, ...recompute(overrides) };
    }),

  setOverride: (id, binding) =>
    set((s) => {
      const overrides = { ...s.overrides };
      if (binding === null) delete overrides[id];
      else overrides[id] = binding;
      return { overrides, ...recompute(overrides) };
    }),

  resetCommand: (id) =>
    set((s) => {
      if (!(id in s.overrides)) return {};
      const overrides = { ...s.overrides };
      delete overrides[id];
      return { overrides, ...recompute(overrides) };
    }),

  resetAll: () => set({ overrides: {}, ...recompute({}) }),
}));

/** Imperative read for non-React call sites (dispatcher, handlers). */
export function getBinding(id: string): string | null {
  return useKeybindingStore.getState().resolved[id] ?? null;
}
