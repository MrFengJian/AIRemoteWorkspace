import { useCallback } from "react";

import { useConfirmStore, type ConfirmRequest } from "@/stores/confirm.store";

/**
 * useConfirm exposes styled confirmation/prompt dialogs as Promises,
 * replacing native window.confirm / window.prompt.
 *
 *   const { askConfirm, askPrompt } = useConfirm();
 *   if (!(await askConfirm("Delete this host?"))) return;
 *   const name = await askPrompt("Folder name");
 */
export function useConfirm() {
  const open = useConfirmStore((s) => s.open);

  const askConfirm = useCallback(
    (opts: string | Omit<ConfirmRequest, "mode">) =>
      open(
        typeof opts === "string"
          ? { mode: "confirm", title: opts, message: "" }
          : { mode: "confirm", ...opts },
      ) as Promise<boolean>,
    [open],
  );

  const askPrompt = useCallback(
    (opts: string | Omit<ConfirmRequest, "mode">) =>
      open(
        typeof opts === "string"
          ? { mode: "prompt", title: opts, message: "" }
          : { mode: "prompt", ...opts },
      ) as Promise<string | null>,
    [open],
  );

  return { askConfirm, askPrompt };
}
