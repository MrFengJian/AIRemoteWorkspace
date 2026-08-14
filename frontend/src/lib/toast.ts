import { useToastStore, type ToastVariant } from "@/stores/toast.store";

/**
 * Toast helpers — callable from anywhere (components, hooks, plain modules):
 *
 *   toast.success("Host saved");
 *   toast.error(err instanceof Error ? err.message : String(err));
 *
 * Rendered by the single <Toaster /> host in AppShell.
 */
function show(variant: ToastVariant, message: unknown) {
  const text =
    message instanceof Error
      ? message.message
      : typeof message === "string"
        ? message
        : String(message);
  if (!text) return;
  useToastStore.getState().push(variant, text);
}

export const toast = {
  success: (message: unknown) => show("success", message),
  error: (message: unknown) => show("error", message),
  info: (message: unknown) => show("info", message),
};

/** Extract a human-readable message from an unknown thrown value. */
export function errorMessage(e: unknown): string {
  if (e instanceof Error) return e.message;
  if (typeof e === "string" && e) return e;
  try {
    return JSON.stringify(e);
  } catch {
    return String(e);
  }
}
