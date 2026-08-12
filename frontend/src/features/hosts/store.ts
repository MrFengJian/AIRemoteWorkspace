import { create } from "zustand";

import type { HostDTO } from "@/features/hosts/api";

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
  openEditor: (host: HostDTO | "new") => void;
  closeEditor: () => void;
}

export const useHostsUIStore = create<HostsUIState>((set) => ({
  selectedId: null,
  select: (id) => set({ selectedId: id }),

  editing: null,
  openEditor: (host) => set({ editing: host }),
  closeEditor: () => set({ editing: null }),
}));
