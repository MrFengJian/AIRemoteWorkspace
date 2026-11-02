import { create } from "zustand";

/**
 * Top-level navigation views. The terminal workspace IS the app — the hosts
 * sidebar, session tabs, SFTP and the agent all live inside it. Settings and
 * the fault-report tracker are separate pages. "sftp" is used by the
 * standalone SFTP window (the stores are per-webview, so it never leaks into
 * the main window).
 */
export type AppView = "terminal" | "settings" | "faults" | "sftp" | "apps";

/** Settings sidebar categories. Global so other views can deep-link into one. */
export type SettingsCategory =
  | "appearance"
  | "language"
  | "models"
  | "experts"
  | "agent"
  | "mcp"
  | "shortcuts"
  | "keys"
  | "advanced"
  | "about";

/** 跨视图跳转意图：从应用管理页选择主机后，跳到会话右侧应用面板并预选应用。 */
export interface PendingAppIntent {
  appId: string;
  hostId: string;
}

interface UIState {
  activeView: AppView;
  setView: (view: AppView) => void;
  settingsCategory: SettingsCategory;
  setSettingsCategory: (category: SettingsCategory) => void;
  /** 待处理的应用跳转意图（TerminalView 消费后置空）。 */
  pendingApp: PendingAppIntent | null;
  setPendingApp: (intent: PendingAppIntent | null) => void;
}

/**
 * Global UI state that is not owned by any single feature
 * (active navigation; toasts live in stores/toast.store.ts).
 *
 * Feature-local state lives in `features/<name>/store.ts`.
 */
export const useUIStore = create<UIState>((set) => ({
  activeView: "terminal",
  setView: (view) => set({ activeView: view }),
  settingsCategory: "appearance",
  setSettingsCategory: (category) => set({ settingsCategory: category }),
  pendingApp: null,
  setPendingApp: (intent) => set({ pendingApp: intent }),
}));
