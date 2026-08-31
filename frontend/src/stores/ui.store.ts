import { create } from "zustand";

/**
 * Top-level navigation views. The terminal workspace IS the app — the hosts
 * sidebar, session tabs, SFTP and the agent all live inside it. Settings is
 * the only separate page. "sftp" is used by the standalone SFTP window (the
 * stores are per-webview, so it never leaks into the main window).
 */
export type AppView = "terminal" | "settings" | "sftp";

/** Settings sidebar categories. Global so other views can deep-link into one. */
export type SettingsCategory =
  | "appearance"
  | "language"
  | "models"
  | "agent"
  | "shortcuts"
  | "advanced"
  | "about";

interface UIState {
  activeView: AppView;
  setView: (view: AppView) => void;
  settingsCategory: SettingsCategory;
  setSettingsCategory: (category: SettingsCategory) => void;
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
}));
