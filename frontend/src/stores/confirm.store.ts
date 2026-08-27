import { create } from "zustand";

/**
 * Global confirmation / prompt dialog state.
 *
 * Replaces native window.confirm / window.prompt with a styled dialog.
 * Components open a dialog via useConfirm() (askConfirm / askPrompt) which
 * returns a Promise; the single ConfirmDialog host (mounted in AppShell)
 * renders the overlay and resolves the pending promise on user action.
 */

export type ConfirmMode = "confirm" | "prompt";

export interface ConfirmRequest {
  mode: ConfirmMode;
  title: string;
  /** Optional body text (shown under the title). */
  message?: string;
  /** Confirm button label (default "Confirm"). */
  confirmLabel?: string;
  /** Cancel button label (default "Cancel"). */
  cancelLabel?: string;
  /** Optional third (middle) button; resolves the promise with "alt". */
  altLabel?: string;
  /** Red / destructive styling for the confirm button. */
  danger?: boolean;
  /** Placeholder for prompt inputs. */
  placeholder?: string;
  /** Initial value for prompt inputs. */
  initialValue?: string;
}

interface ConfirmState {
  request: ConfirmRequest | null;
  resolve: ((value: boolean | string | null) => void) | null;
  /** Opens a dialog and resolves with the user's choice. */
  open: (req: ConfirmRequest) => Promise<boolean | string | null>;
  close: () => void;
}

export const useConfirmStore = create<ConfirmState>((set) => ({
  request: null,
  resolve: null,

  open: (req) =>
    new Promise((resolve) => {
      set({ request: req, resolve });
    }),

  close: () => set({ request: null, resolve: null }),
}));
