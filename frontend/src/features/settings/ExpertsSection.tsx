import { useState } from "react";
import { useTranslation } from "react-i18next";
import { useQueryClient } from "@tanstack/react-query";
import { Copy, Loader2, Pencil, Plus, Trash2, UsersRound } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { useConfirm } from "@/lib/useConfirm";
import { toast } from "@/lib/toast";
import {
  useExperts,
  useSaveExpert,
  useDeleteExpert,
  EXPERTS_KEY,
} from "@/features/experts/hooks";
import type { ExpertDTO } from "@/features/experts/api";
import { ExpertAvatar } from "@/features/experts/avatar";
import { ExpertFormDialog } from "@/features/settings/ExpertFormDialog";

/**
 * Settings → 数字员工 (Experts). Manages the digital-employee roster: the
 * builtin personas seeded from the binary plus user-defined ones. List +
 * mutations run through TanStack Query (queryKey "experts") so the agent
 * panel's persona picker stays in sync. Deleting a builtin only dismisses it
 * (editable copies are preserved; the persona can be restored by re-saving).
 */
export function ExpertsSection() {
  const { t } = useTranslation();
  const { askConfirm } = useConfirm();
  const queryClient = useQueryClient();

  const { data: experts, isLoading } = useExperts();
  const saveExpert = useSaveExpert();
  const deleteExpert = useDeleteExpert();

  const [dialogOpen, setDialogOpen] = useState(false);
  const [editing, setEditing] = useState<ExpertDTO | null>(null);
  const [duplicating, setDuplicating] = useState<ExpertDTO | null>(null);

  const openAdd = () => {
    setEditing(null);
    setDuplicating(null);
    setDialogOpen(true);
  };

  const openEdit = (e: ExpertDTO) => {
    setEditing(e);
    setDuplicating(null);
    setDialogOpen(true);
  };

  /** Fork a copy: cleared id → the backend assigns a fresh one, so the new
   *  expert starts as a custom persona with the same shape. */
  const openDuplicate = (e: ExpertDTO) => {
    setEditing(null);
    setDuplicating({ ...e, id: "", name: t("settings.experts.copyName", { name: e.name }), builtin: false });
    setDialogOpen(true);
  };

  const handleToggle = (e: ExpertDTO, enabled: boolean) => {
    saveExpert.mutate({ ...e, enabled }, {
      onError: (err) => toast.error(errorMessage(err)),
    });
  };

  const handleDelete = async (e: ExpertDTO) => {
    const ok = await askConfirm({
      title: t("settings.experts.deleteTitle"),
      message: e.builtin
        ? t("settings.experts.deleteBuiltinConfirm", { name: e.name })
        : t("settings.experts.deleteConfirm", { name: e.name }),
      danger: true,
      confirmLabel: t("common.delete"),
    });
    if (!ok) return;
    deleteExpert.mutate(e.id, {
      onError: (err) => toast.error(errorMessage(err)),
    });
  };

  return (
    <div className="flex flex-col gap-4">
      <div className="flex items-center justify-between">
        <h2 className="text-lg font-semibold">{t("settings.experts.title")}</h2>
        <Button size="sm" onClick={openAdd}>
          <Plus className="h-3.5 w-3.5" />
          {t("settings.experts.add")}
        </Button>
      </div>

      <Card>
        <CardHeader>
          <CardTitle className="text-sm">{t("settings.experts.roster")}</CardTitle>
          <CardDescription>{t("settings.experts.rosterDesc")}</CardDescription>
        </CardHeader>
        <CardContent className="flex flex-col gap-2">
          {isLoading ? (
            <div className="flex items-center justify-center py-8 text-muted-foreground">
              <Loader2 className="h-4 w-4 animate-spin" />
            </div>
          ) : !experts || experts.length === 0 ? (
            <div className="flex flex-col items-center gap-2 py-8 text-center">
              <UsersRound className="h-8 w-8 text-muted-foreground" />
              <p className="text-sm text-muted-foreground">{t("settings.experts.empty")}</p>
              <Button size="sm" variant="outline" onClick={openAdd}>
                <Plus className="h-3.5 w-3.5" />
                {t("settings.experts.add")}
              </Button>
            </div>
          ) : (
            experts.map((e) => (
              <div
                key={e.id}
                className="flex flex-col gap-2 rounded-[var(--radius)] border border-border p-3"
              >
                <div className="flex items-center gap-2.5">
                  <ExpertAvatar icon={e.icon} color={e.color} className="h-8 w-8" iconClassName="h-4 w-4" />
                  <div className="min-w-0 flex-1">
                    <div className="flex items-center gap-2">
                      <span className="truncate text-sm font-medium">{e.name}</span>
                      {e.builtin && <Badge variant="secondary">{t("settings.experts.builtin")}</Badge>}
                      {e.autoSnapshot && <Badge variant="outline">{t("settings.experts.autoSnapshot")}</Badge>}
                      {!e.enabled && <Badge variant="secondary">{t("settings.experts.disabled")}</Badge>}
                    </div>
                    <p className="mt-0.5 truncate text-xs text-muted-foreground">
                      {e.role ? `${e.role} · ` : ""}{e.description}
                    </p>
                  </div>
                  <Checkbox
                    checked={e.enabled}
                    onCheckedChange={(v) => handleToggle(e, v === true)}
                    disabled={saveExpert.isPending}
                    title={t("settings.experts.toggle")}
                  />
                </div>
                <div className="flex items-center gap-1">
                  <Button variant="ghost" size="sm" className="h-7" onClick={() => openEdit(e)}>
                    <Pencil className="h-3.5 w-3.5" />
                    {t("common.edit")}
                  </Button>
                  <Button variant="ghost" size="sm" className="h-7" onClick={() => openDuplicate(e)}>
                    <Copy className="h-3.5 w-3.5" />
                    {t("settings.experts.duplicate")}
                  </Button>
                  <Button
                    variant="ghost"
                    size="sm"
                    className="h-7 text-destructive hover:text-destructive"
                    onClick={() => handleDelete(e)}
                    disabled={deleteExpert.isPending}
                  >
                    <Trash2 className="h-3.5 w-3.5" />
                    {t("common.delete")}
                  </Button>
                </div>
              </div>
            ))
          )}
        </CardContent>
      </Card>

      <ExpertFormDialog
        open={dialogOpen}
        expert={editing ?? duplicating}
        onClose={() => {
          setDialogOpen(false);
          setEditing(null);
          setDuplicating(null);
        }}
        onSaved={() => {
          queryClient.invalidateQueries({ queryKey: EXPERTS_KEY });
        }}
      />
    </div>
  );
}

function errorMessage(e: unknown): string {
  return e instanceof Error ? e.message : String(e);
}
