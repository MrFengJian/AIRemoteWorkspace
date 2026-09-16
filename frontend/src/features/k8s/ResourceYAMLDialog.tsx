import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { Copy, FileCode, RotateCw, Save } from "lucide-react";

import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { k8sApi } from "@/features/k8s/api";
import { useConfirm } from "@/lib/useConfirm";
import { toast } from "@/lib/toast";
import { cn } from "@/lib/utils";

export interface YamlTarget {
  /** kubectl singular kind (deployment/statefulset/daemonset/pod/service). */
  kind: string;
  namespace: string;
  name: string;
}

interface ResourceYAMLDialogProps {
  open: boolean;
  target: YamlTarget | null;
  /** Session id for the backend exec channel. */
  sessionID: string;
  onClose: () => void;
}

/**
 * ResourceYAMLDialog — view and edit one resource's live manifest. Loads
 * `kubectl get -o yaml` into an editable textarea; "保存并应用" pipes the
 * edited document back through `kubectl apply -f -` (backend guards verify
 * the document's kind/name/namespace still match the target, so a mangled
 * edit is rejected instead of creating some other object). The apply button
 * stays disabled until the text differs from the loaded manifest.
 */
export function ResourceYAMLDialog({ open, target, sessionID, onClose }: ResourceYAMLDialogProps) {
  const { t } = useTranslation();
  const { askConfirm } = useConfirm();
  const queryClient = useQueryClient();
  const [text, setText] = useState("");
  const [original, setOriginal] = useState("");

  const kind = target?.kind ?? "";
  const namespace = target?.namespace ?? "";
  const name = target?.name ?? "";

  const yamlQ = useQuery({
    queryKey: ["k8s-yaml", sessionID, kind, namespace, name],
    queryFn: () => k8sApi.resourceYaml(sessionID, kind, namespace, name),
    enabled: open && !!target,
    // Always fresh when opening for edit — a cached manifest could be
    // stale against the cluster and clobber a newer change.
    staleTime: 0,
  });

  // Load the fetched manifest into the editor (also resets after reload).
  useEffect(() => {
    if (yamlQ.data !== undefined) {
      setText(yamlQ.data);
      setOriginal(yamlQ.data);
    }
  }, [yamlQ.data]);

  // Switching targets resets the editor to the (old) cached text instantly;
  // the fresh fetch above then replaces it.
  useEffect(() => {
    setText("");
    setOriginal("");
  }, [kind, namespace, name]);

  const applyMut = useMutation({
    mutationFn: () => k8sApi.applyResourceYaml(sessionID, kind, namespace, name, text),
    meta: { silent: true }, // errors render inline, next to the document
    onSuccess: async () => {
      toast.success(t("k8s.yamlApplied", { name }));
      for (const key of [["k8s-workloads"], ["k8s-pods"], ["k8s-services"], ["k8s-info"], ["k8s-events"]]) {
        await queryClient.invalidateQueries({ queryKey: [key[0], sessionID] });
      }
      onClose();
    },
  });

  const dirty = text !== original && text.trim() !== "";
  const busy = applyMut.isPending;

  const apply = async () => {
    if (!dirty) return;
    const ok = await askConfirm({
      title: t("k8s.yamlApply"),
      message: t("k8s.yamlApplyConfirm", { kind, name }),
      danger: true,
      confirmLabel: t("k8s.yamlApply"),
    });
    if (!ok) return;
    applyMut.mutate();
  };

  const copyYaml = async () => {
    try {
      await navigator.clipboard.writeText(text);
      toast.info(t("k8s.copied"));
    } catch {
      /* clipboard unavailable */
    }
  };

  const applyErr = applyMut.isError
    ? String((applyMut.error as Error)?.message ?? applyMut.error)
    : "";

  return (
    <Dialog open={open} onOpenChange={(v) => !v && onClose()}>
      <DialogContent className="flex h-[85vh] max-w-3xl flex-col">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <FileCode className="h-4 w-4 text-primary" />
            <span className="font-mono">{name}</span>
            <span className="shrink-0 rounded-full border border-border px-1.5 py-px text-[10px] text-muted-foreground">
              {kind}
            </span>
            {namespace && (
              <span className="shrink-0 rounded bg-muted/50 px-1 font-mono text-[10px] text-muted-foreground">
                {namespace}
              </span>
            )}
          </DialogTitle>
          <DialogDescription>{t("k8s.yamlHint")}</DialogDescription>
        </DialogHeader>

        <div className="flex min-h-0 flex-1 flex-col">
          {yamlQ.isError ? (
            <div
              className="flex-1 overflow-auto rounded-[var(--radius)] border border-destructive/40 bg-destructive/10 p-3 text-xs text-destructive"
              title={String((yamlQ.error as Error)?.message ?? "")}
            >
              {t("k8s.yamlLoadFailed")}: {String((yamlQ.error as Error)?.message ?? "")}
            </div>
          ) : (
            <textarea
              value={text}
              onChange={(e) => setText(e.target.value)}
              spellCheck={false}
              disabled={yamlQ.isLoading}
              placeholder={t("common.loading")}
              aria-label={t("k8s.viewYaml")}
              className="min-h-0 flex-1 resize-none rounded-[var(--radius)] border border-input bg-background p-2.5 font-mono text-[11px] leading-relaxed text-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring disabled:opacity-60"
            />
          )}

          {applyErr && (
            <div
              className="mt-2 max-h-24 shrink-0 overflow-auto rounded border border-destructive/40 bg-destructive/10 px-2 py-1.5 text-[11px] text-destructive"
              title={applyErr}
            >
              {t("k8s.yamlApplyFailed")}: {applyErr}
            </div>
          )}
        </div>

        <DialogFooter className="flex shrink-0 items-center gap-1.5 border-t border-border pt-3">
          <span className="mr-auto text-[11px] text-muted-foreground">
            {dirty ? t("k8s.yamlDirty") : ""}
          </span>
          <Button type="button" variant="ghost" size="sm" onClick={() => void copyYaml()} disabled={!text}>
            <Copy className="h-3.5 w-3.5" />
            {t("common.copy")}
          </Button>
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={() => void yamlQ.refetch()}
            disabled={yamlQ.isFetching}
            title={t("k8s.yamlReload")}
          >
            <RotateCw className={cn("h-3.5 w-3.5", yamlQ.isFetching && "animate-spin")} />
            {t("k8s.yamlReload")}
          </Button>
          <Button type="button" size="sm" onClick={() => void apply()} disabled={!dirty || busy}>
            <Save className="h-3.5 w-3.5" />
            {t("k8s.yamlApply")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
