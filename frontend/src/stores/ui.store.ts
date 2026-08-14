import { create } from "zustand";

/**
 * Top-level navigation views. Each maps to a feature area
 * (AGENT.md §6 Feature-Based Architecture).
 *
 * `dashboard` is the landing view shown at startup; the others light up as
 * their owning feature lands in later phases.
 */
export type AppView =
  | "dashboard"
  | "hosts"
  | "terminal"
  | "sftp"
  | "agent"
  | "settings";

/** Settings sidebar categories. Global so other views can deep-link into one. */
export type SettingsCategory =
  | "appearance"
  | "language"
  | "models"
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
 * (active navigation, future: command-palette open, global toasts).
 *
 * Feature-local state lives in `features/<name>/store.ts`.
 */
export const useUIStore = create<UIState>((set) => ({
  activeView: "dashboard",
  setView: (view) => set({ activeView: view }),
  settingsCategory: "appearance",
  setSettingsCategory: (category) => set({ settingsCategory: category }),
}));
