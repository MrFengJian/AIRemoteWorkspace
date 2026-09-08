import { create } from "zustand";

import type { HostDTO } from "@/features/hosts/api";

/**
 * Host form dialog tab groups, Xshell-style. Lives here (not in
 * HostFormDialog) so callers can deep-link the editor to a specific tab —
 * e.g. the tunnel panel opens a host straight on its tunnel rules.
 */
export type HostFormTab = "connection" | "appearance" | "organisation" | "tunnel";

/**
 * Hosts feature UI state (local-only concerns).
 *
 * The host *list* is server state owned by TanStack Query (useHosts hook);
 * this store holds transient UI state like which host the connect dialog is
 * for, and the selected host id.
 */
interface HostsUIState {
  /** Host currently selected in the list (for connect/delete actions). */
  selectedId: string | null;
  select: (id: string | null) => void;

  /** Host being edited in the form dialog, null when closed, undefined = new. */
  editing: HostDTO | "new" | null;
  /** Tab the form dialog should open on; null = its default ("connection"). */
  editingTab: HostFormTab | null;
  openEditor: (host: HostDTO | "new", tab?: HostFormTab) => void;
  closeEditor: () => void;
}

export const useHostsUIStore = create<HostsUIState>((set) => ({
  selectedId: null,
  select: (id) => set({ selectedId: id }),

  editing: null,
  editingTab: null,
  openEditor: (host, tab) => set({ editing: host, editingTab: tab ?? null }),
  closeEditor: () => set({ editing: null, editingTab: null }),
}));
