import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import {
  ArrowDown,
  ArrowUp,
  CornerDownLeft,
  Pencil,
  Plus,
  SquareTerminal,
  Trash2,
} from "lucide-react";

import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Checkbox } from "@/components/ui/checkbox";
import { useConfirmStore } from "@/stores/confirm.store";
import {
  useQuickCommands,
  saveQuickCommands,
  type QuickCommand,
} from "@/features/terminal/quickCommands";
import { errorMessage, toast } from "@/lib/toast";

interface Draft {
  /** Index into the list, or -1 when creating a new entry. */
  index: number;
  name: string;
  command: string;
  sendEnter: boolean;
}

/**
 * Manage the quick command bar entries: add, edit, delete and reorder.
 * Every change persists immediately (read-modify-write on AppConfig), so the
 * bar below the terminal updates live through the "config:changed" event.
 */
export function QuickCommandManageDialog({ onClose }: { onClose: () => void }) {
  const { t } = useTranslation();
  const commands = useQuickCommands();
  const askConfirm = useConfirmStore((s) => s.open);

  // Local working copy so in-dialog edits (reorder/delete) render instantly;
  // persisted through saveList below. Re-synced when the underlying config
  // changes from elsewhere (useQuickCommands listens to "config:changed") —
  // but not while a draft edit is open, so a concurrent save can't wipe it.
  const [list, setList] = useState<QuickCommand[]>(commands);
  const [draft, setDraft] = useState<Draft | null>(null);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (!draft) setList(commands);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [commands]);

  /** Persist `next` and mirror it into the local list. */
  const saveList = async (next: QuickCommand[]) => {
    setSaving(true);
    try {
      await saveQuickCommands(next);
      setList(next);
    } catch (e) {
      toast.error(`${t("quickCmd.saveFailed")}: ${errorMessage(e)}`);
    } finally {
      setSaving(false);
    }
  };

  const saveDraft = async () => {
    if (!draft || draft.name.trim() === "" || draft.command.trim() === "") return;
    const entry: QuickCommand = {
      id: draft.index >= 0 && list[draft.index] ? list[draft.index].id : crypto.randomUUID(),
      name: draft.name.trim(),
      command: draft.command,
      sendEnter: draft.sendEnter,
    };
    const next = [...list];
    if (draft.index >= 0) next[draft.index] = entry;
    else next.push(entry);
    setDraft(null);
    await saveList(next);
  };

  const removeAt = async (index: number) => {
    const entry = list[index];
    if (!entry) return;
    const ok = await askConfirm({
      mode: "confirm",
      title: t("quickCmd.deleteTitle"),
      message: t("quickCmd.deleteMsg", { name: entry.name }),
      danger: true,
    });
    if (!ok) return;
    await saveList(list.filter((_, i) => i !== index));
  };

  const move = async (index: number, dir: -1 | 1) => {
    const to = index + dir;
    if (to < 0 || to >= list.length) return;
    const next = [...list];
    [next[index], next[to]] = [next[to], next[index]];
    await saveList(next);
  };

  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="max-w-xl">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2 text-base">
            <SquareTerminal className="h-4 w-4 text-primary" />
            {t("quickCmd.manage")}
          </DialogTitle>
          <DialogDescription>{t("quickCmd.manageDesc")}</DialogDescription>
        </DialogHeader>

        {draft === null ? (
          <>
            <div className="max-h-72 overflow-y-auto">
              {list.length === 0 ? (
                <p className="py-8 text-center text-sm text-muted-foreground">
                  {t("quickCmd.emptyHint")}
                </p>
              ) : (
                <div className="flex flex-col gap-1">
                  {list.map((cmd, i) => (
                    <div
                      key={cmd.id}
                      className="group flex items-center gap-2 rounded-[var(--radius)] border border-border px-2 py-1.5"
                    >
                      <div className="min-w-0 flex-1">
                        <div className="flex items-center gap-1.5 text-xs font-medium text-foreground">
                          <span className="truncate">{cmd.name}</span>
                          {cmd.sendEnter && (
                            <span title={t("quickCmd.sendEnter")}>
                              <CornerDownLeft className="h-3 w-3 text-muted-foreground" />
                            </span>
                          )}
                        </div>
                        <div className="truncate font-mono text-[11px] text-muted-foreground">
                          {cmd.command.replace(/\n/g, " ⏎ ")}
                        </div>
                      </div>
                      <div className="flex shrink-0 items-center opacity-0 transition-opacity group-hover:opacity-100">
                        <button
                          type="button"
                          disabled={i === 0 || saving}
                          onClick={() => void move(i, -1)}
                          title={t("quickCmd.moveUp")}
                          className="rounded p-1 text-muted-foreground hover:bg-accent hover:text-foreground disabled:opacity-30"
                        >
                          <ArrowUp className="h-3.5 w-3.5" />
                        </button>
                        <button
                          type="button"
                          disabled={i === list.length - 1 || saving}
                          onClick={() => void move(i, 1)}
                          title={t("quickCmd.moveDown")}
                          className="rounded p-1 text-muted-foreground hover:bg-accent hover:text-foreground disabled:opacity-30"
                        >
                          <ArrowDown className="h-3.5 w-3.5" />
                        </button>
                        <button
                          type="button"
                          onClick={() =>
                            setDraft({
                              index: i,
                              name: cmd.name,
                              command: cmd.command,
                              sendEnter: cmd.sendEnter,
                            })
                          }
                          title={t("common.edit")}
                          className="rounded p-1 text-muted-foreground hover:bg-accent hover:text-foreground"
                        >
                          <Pencil className="h-3.5 w-3.5" />
                        </button>
                        <button
                          type="button"
                          disabled={saving}
                          onClick={() => void removeAt(i)}
                          title={t("common.delete")}
                          className="rounded p-1 text-destructive hover:bg-destructive/10 disabled:opacity-30"
                        >
                          <Trash2 className="h-3.5 w-3.5" />
                        </button>
                      </div>
                    </div>
                  ))}
                </div>
              )}
            </div>

            <DialogFooter>
              <Button variant="outline" onClick={onClose}>
                {t("common.close")}
              </Button>
              <Button
                className="bg-primary text-primary-foreground"
                disabled={saving}
                onClick={() =>
                  setDraft({ index: -1, name: "", command: "", sendEnter: true })
                }
              >
                <Plus className="h-4 w-4" /> {t("quickCmd.add")}
              </Button>
            </DialogFooter>
          </>
        ) : (
          <>
            <div className="flex flex-col gap-3">
              <div className="grid grid-cols-[4.5rem_1fr] items-center gap-3">
                <span className="text-sm text-muted-foreground">{t("quickCmd.name")}</span>
                <Input
                  autoFocus
                  value={draft.name}
                  onChange={(e) => setDraft({ ...draft, name: e.target.value })}
                  placeholder={t("quickCmd.namePlaceholder")}
                />
              </div>
              <div className="grid grid-cols-[4.5rem_1fr] items-start gap-3">
                <span className="pt-1.5 text-sm text-muted-foreground">
                  {t("quickCmd.command")}
                </span>
                <textarea
                  value={draft.command}
                  onChange={(e) => setDraft({ ...draft, command: e.target.value })}
                  placeholder={t("quickCmd.commandPlaceholder")}
                  rows={5}
                  className="w-full rounded-[calc(var(--radius)-2px)] border border-input bg-background px-2.5 py-1.5 font-mono text-xs focus:outline-none focus:ring-1 focus:ring-ring"
                />
              </div>
              <div className="grid grid-cols-[4.5rem_1fr] items-start gap-3">
                <span className="pt-0.5 text-sm text-muted-foreground">
                  {t("quickCmd.sendEnter")}
                </span>
                <div className="flex flex-col gap-1">
                  <label className="flex cursor-pointer items-center gap-2 text-sm">
                    <Checkbox
                      checked={draft.sendEnter}
                      onCheckedChange={(v) => setDraft({ ...draft, sendEnter: v === true })}
                    />
                    {t("quickCmd.sendEnterLabel")}
                  </label>
                  <p className="text-[11px] text-muted-foreground">
                    {t("quickCmd.sendEnterHint")}
                  </p>
                </div>
              </div>
            </div>

            <DialogFooter>
              <Button variant="outline" onClick={() => setDraft(null)}>
                {t("common.cancel")}
              </Button>
              <Button
                className="bg-primary text-primary-foreground"
                disabled={
                  draft.name.trim() === "" ||
                  draft.command.trim() === "" ||
                  saving
                }
                onClick={() => void saveDraft()}
              >
                {t("common.save")}
              </Button>
            </DialogFooter>
          </>
        )}
      </DialogContent>
    </Dialog>
  );
}
