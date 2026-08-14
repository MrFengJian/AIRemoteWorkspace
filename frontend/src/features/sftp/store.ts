import { create } from "zustand";

import type { FileEntryDTO } from "@/features/sftp/api";

/**
 * SFTP feature UI state: which host is being browsed, the current remote path,
 * the fetched entries, and load/error status.
 *
 * The list itself is refetched on demand (navigate/upload/delete) rather than
 * held in TanStack Query, because SFTP listings mutate frequently and a
 * simple manual refresh keeps the mental model clear.
 */
interface SftpState {
  hostId: string | null;
  hostName: string | null;
  cwd: string;
  entries: FileEntryDTO[];
  loading: boolean;
  error: string | null;
  /** Whether dotfiles are listed. Off by default; toggled from the toolbar. */
  showHidden: boolean;

  setHost: (id: string | null, name: string | null) => void;
  setCwd: (dir: string) => void;
  setEntries: (entries: FileEntryDTO[]) => void;
  setLoading: (v: boolean) => void;
  setError: (msg: string | null) => void;
  setShowHidden: (v: boolean) => void;
}

export const useSftpStore = create<SftpState>((set) => ({
  hostId: null,
  hostName: null,
  cwd: "/",
  entries: [],
  loading: false,
  error: null,
  showHidden: false,

  setHost: (id, name) =>
    set({ hostId: id, hostName: name, cwd: "/", entries: [], error: null }),
  setCwd: (dir) => set({ cwd: dir, error: null }),
  setEntries: (entries) => set({ entries }),
  setLoading: (v) => set({ loading: v }),
  setError: (msg) => set({ error: msg, loading: false }),
  setShowHidden: (v) => set({ showHidden: v }),
}));
