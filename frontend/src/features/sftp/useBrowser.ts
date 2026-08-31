// useBrowser — browse state for one SFTP panel pane (local or remote):
// current directory, listing, hidden-files toggle and manual refresh. All
// mutations go through the backend; this hook only reads and navigates.

import { useCallback, useEffect, useState } from "react";

import type { FileEntryDTO } from "@/features/sftp/api";
import type { Side } from "@/features/sftp/clipboard";

export interface BrowserState {
  side: Side;
  cwd: string;
  entries: FileEntryDTO[];
  loading: boolean;
  error: string | null;
  showHidden: boolean;
  setShowHidden: (v: boolean) => void;
  /** Navigate to a directory (listing refreshes automatically). */
  navigate: (dir: string) => void;
  /** Re-list the current directory. */
  refresh: () => void;
  /** Force a fresh listing of dir, clearing stale rows (host switch). */
  reset: (dir: string) => void;
}

/**
 * @param side    which pane this state drives (label + path semantics)
 * @param list    directory listing call for this pane's side
 * @param initial resolves the starting directory (home for local, "/" remote)
 */
export function useBrowser(
  side: Side,
  list: (dir: string) => Promise<FileEntryDTO[]>,
  initial: () => Promise<string>,
): BrowserState {
  const [cwd, setCwd] = useState("");
  const [entries, setEntries] = useState<FileEntryDTO[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [showHidden, setShowHidden] = useState(false);
  const [tick, setTick] = useState(0);

  // Resolve the starting directory once ("", until then, lists nothing).
  useEffect(() => {
    let alive = true;
    initial()
      .then((dir) => {
        if (alive) setCwd(dir);
      })
      .catch(() => {
        if (alive) setCwd("/");
      });
    return () => {
      alive = false;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // List whenever the directory changes, the listing call changes (host
  // switch) or a refresh is requested.
  useEffect(() => {
    if (!cwd) return;
    let alive = true;
    setLoading(true);
    setError(null);
    list(cwd)
      .then((rows) => {
        if (alive) setEntries(rows);
      })
      .catch((e) => {
        if (alive) {
          setError(e instanceof Error ? e.message : String(e));
          setEntries([]);
        }
      })
      .finally(() => {
        if (alive) setLoading(false);
      });
    return () => {
      alive = false;
    };
  }, [cwd, tick, list]);

  const navigate = useCallback((dir: string) => setCwd(dir), []);
  const refresh = useCallback(() => setTick((t) => t + 1), []);
  const reset = useCallback((dir: string) => {
    setEntries([]);
    setCwd(dir);
    setTick((t) => t + 1);
  }, []);

  return {
    side,
    cwd,
    entries,
    loading,
    error,
    showHidden,
    setShowHidden,
    navigate,
    refresh,
    reset,
  };
}
