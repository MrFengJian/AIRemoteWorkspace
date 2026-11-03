import { create } from "zustand";

/**
 * Quick links — the user's pinned hosts, shown as one-click connect chips in
 * the bottom bar. Pure frontend preference state: localStorage persistence is
 * the storage of record (host ids only; the id list is resolved against the
 * live host list at render time, so stale ids of deleted hosts just vanish).
 */
const STORAGE_KEY = "workspace.quickLinks";

function load(): string[] {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (!raw) return [];
    const parsed: unknown = JSON.parse(raw);
    return Array.isArray(parsed) ? parsed.filter((x): x is string => typeof x === "string") : [];
  } catch {
    return [];
  }
}

function persist(ids: string[]) {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(ids));
  } catch {
    /* storage unavailable — links stay in memory for this run */
  }
}

interface QuickLinksState {
  /** Pinned host ids, in pin order. */
  ids: string[];
  /** Pin a host (appends) or unpin it. */
  toggle: (hostID: string) => void;
  /** Unpin a host (no-op when absent). */
  remove: (hostID: string) => void;
}

export const useQuickLinksStore = create<QuickLinksState>((set) => ({
  ids: load(),
  toggle: (hostID) =>
    set((s) => {
      const ids = s.ids.includes(hostID) ? s.ids.filter((x) => x !== hostID) : [...s.ids, hostID];
      persist(ids);
      return { ids };
    }),
  remove: (hostID) =>
    set((s) => {
      const ids = s.ids.filter((x) => x !== hostID);
      if (ids.length === s.ids.length) return s;
      persist(ids);
      return { ids };
    }),
}));
