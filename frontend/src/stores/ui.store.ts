import { create } from "zustand";

/**
 * Top-level navigation views. Each maps to a feature area
 * (AGENT.md §6 Feature-Based Architecture).
 *
 * `hosts` is the landing view shown at startup. The agent and the SFTP file
 * browser are deliberately absent: both are session-scoped and live as
 * embedded panels inside the terminal view, not as standalone pages.
 */
export type AppView = "hosts" | "terminal" | "settings";

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
 * (active navigation; toasts live in stores/toast.store.ts).
 *
 * Feature-local state lives in `features/<name>/store.ts`.
 */
export const useUIStore = create<UIState>((set) => ({
  activeView: "hosts",
  setView: (view) => set({ activeView: view }),
  settingsCategory: "appearance",
  setSettingsCategory: (category) => set({ settingsCategory: category }),
}));
