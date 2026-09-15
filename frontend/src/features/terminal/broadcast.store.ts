import { create } from "zustand";

/**
 * 同步键入 (broadcast input) state: when enabled, keystrokes typed in the
 * focused terminal pane are mirrored in real time to the target tabs. The
 * QuickCommandBar owns the toggle and the target selection (its multi-select
 * drives the target list); TerminalPanel reads the store at keystroke time.
 */
interface BroadcastState {
  enabled: boolean;
  /** Tab ids receiving the mirrored input (empty = broadcast does nothing). */
  targetIds: string[];
  setEnabled: (enabled: boolean) => void;
  setTargets: (targetIds: string[]) => void;
}

export const useBroadcastStore = create<BroadcastState>((set) => ({
  enabled: false,
  targetIds: [],
  setEnabled: (enabled) => set({ enabled }),
  setTargets: (targetIds) => set({ targetIds }),
}));
