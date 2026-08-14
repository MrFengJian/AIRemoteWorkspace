import { create } from "zustand";

/**
 * Global toast notification state.
 *
 * Mirrors the confirm-dialog architecture: a zustand store driven by
 * `toast.success/error/info()` helpers (see lib/toast.ts), rendered by the
 * single <Toaster /> host mounted in AppShell. No third-party dependency.
 */
export type ToastVariant = "success" | "error" | "info";

export interface ToastItem {
  id: number;
  variant: ToastVariant;
  message: string;
}

/** Auto-dismiss delays by variant; errors stay longer so they can be read. */
const DISMISS_AFTER: Record<ToastVariant, number> = {
  success: 3000,
  info: 4000,
  error: 5000,
};

/** Maximum toasts visible at once; older ones are dropped. */
const MAX_VISIBLE = 4;

interface ToastState {
  toasts: ToastItem[];
  push: (variant: ToastVariant, message: string) => void;
  dismiss: (id: number) => void;
}

let nextId = 1;

export const useToastStore = create<ToastState>((set) => ({
  toasts: [],

  push: (variant, message) => {
    const id = nextId++;
    set((s) => ({ toasts: [...s.toasts, { id, variant, message }].slice(-MAX_VISIBLE) }));
    window.setTimeout(() => {
      // Dismiss only if still present (manual close may have won the race).
      useToastStore.getState().dismiss(id);
    }, DISMISS_AFTER[variant]);
  },

  dismiss: (id) =>
    set((s) => ({ toasts: s.toasts.filter((t) => t.id !== id) })),
}));
