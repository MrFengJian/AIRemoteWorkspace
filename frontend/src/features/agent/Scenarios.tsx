import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  BookMarked,
  Loader2,
  Pencil,
  Plus,
  Save,
  Sparkles,
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
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { agentApi, type ConversationDTO, type SkillDTO } from "@/features/agent/api";
import { useConfirm } from "@/lib/useConfirm";
import { toast } from "@/lib/toast";

/** Query key shared with AgentView's `/`-picker skills query. */
export const AGENT_SKILLS_KEY = ["agent-skills"] as const;

/** Regex the backend enforces for skill names — mirrored for instant feedback. */
const NAME_RE = /^[a-zA-Z0-9_-]{1,64}$/;

interface EditorState {
  original: string; // "" for a new scenario
  name: string;
  content: string;
}

/**
 * ScenarioManagerDialog — lightweight management UI for diagnosis scenarios
 * (DIAGNOSIS_AGENT.md Phase B): list / create / edit / delete, backed by the
 * skills directory. Builtin packs carry a badge; deleting one stays deleted
 * across restarts (backend dismissed list) until re-created.
 */
export function ScenarioManagerDialog({
  open,
  onOpenChange,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useTranslation();
  const { askConfirm } = useConfirm();
  const queryClient = useQueryClient();
  const [editor, setEditor] = useState<EditorState | null>(null);

  const { data: skills, isLoading } = useQuery({
    queryKey: AGENT_SKILLS_KEY,
    queryFn: agentApi.listSkills,
    enabled: open,
  });

  const refresh = () => {
    queryClient.invalidateQueries({ queryKey: AGENT_SKILLS_KEY }).catch(() => {});
  };

  const handleDelete = async (s: SkillDTO) => {
    const ok = await askConfirm({
      title: t("agent.scenarioDeleteTitle"),
      message: t("agent.scenarioDeleteConfirm", { name: s.name }),
      danger: true,
      confirmLabel: t("common.delete"),
    });
    if (!ok) return;
    try {
      await agentApi.deleteSkill(s.name);
      toast.info(t("agent.scenarioDeleted"));
      refresh();
    } catch (e) {
      toast.error(e instanceof Error ? e.message : String(e));
    }
  };

  const handleSave = async (state: EditorState) => {
    if (!NAME_RE.test(state.name)) {
      toast.error(t("agent.scenarioNameInvalid"));
      return;
    }
    try {
      await agentApi.saveSkill(state.name, state.content);
      toast.info(t("agent.scenarioSaved"));
      setEditor(null);
      refresh();
    } catch (e) {
      toast.error(e instanceof Error ? e.message : String(e));
    }
  };

  return (
    <Dialog
      open={open}
      onOpenChange={(v) => {
        onOpenChange(v);
        if (!v) setEditor(null);
      }}
    >
      <DialogContent className="max-w-xl">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <BookMarked className="h-4 w-4 text-primary" />
            {t("agent.scenarioManagerTitle")}
          </DialogTitle>
          <DialogDescription>{t("agent.scenarioManagerHint")}</DialogDescription>
        </DialogHeader>

        {editor ? (
          <div className="flex flex-col gap-3">
            <div className="flex items-center gap-2">
              <span className="shrink-0 font-mono text-xs text-muted-foreground">/</span>
              <Input
                value={editor.name}
                onChange={(e) => setEditor({ ...editor, name: e.target.value })}
                placeholder="cpu-high"
                aria-label={t("agent.scenarioName")}
                className="h-8 max-w-56 font-mono text-xs"
                disabled={!!editor.original}
              />
            </div>
            <textarea
              value={editor.content}
              onChange={(e) => setEditor({ ...editor, content: e.target.value })}
              rows={14}
              spellCheck={false}
              aria-label={t("agent.scenarioContent")}
              className="w-full resize-y rounded-[var(--radius)] border border-input bg-background p-3 font-mono text-xs leading-relaxed focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
            />
            <DialogFooter>
              <Button type="button" variant="ghost" size="sm" onClick={() => setEditor(null)}>
                {t("common.cancel")}
              </Button>
              <Button
                type="button"
                size="sm"
                onClick={() => void handleSave(editor)}
                disabled={!editor.name.trim() || !editor.content.trim()}
              >
                <Save className="h-3.5 w-3.5" />
                {t("common.save")}
              </Button>
            </DialogFooter>
          </div>
        ) : (
          <>
            <div className="max-h-80 overflow-auto rounded-[var(--radius)] border border-border">
              {isLoading ? (
                <div className="flex items-center justify-center py-8 text-muted-foreground">
                  <Loader2 className="h-4 w-4 animate-spin" />
                </div>
              ) : !skills || skills.length === 0 ? (
                <p className="px-3 py-8 text-center text-xs text-muted-foreground">
                  {t("agent.scenarioEmpty")}
                </p>
              ) : (
                <ul className="divide-y divide-border/60">
                  {skills.map((s) => (
                    <li key={s.name} className="group flex items-start gap-2 px-3 py-2">
                      <div className="min-w-0 flex-1">
                        <p className="flex items-center gap-1.5 text-xs font-medium text-foreground">
                          <span className="font-mono">/{s.name}</span>
                          {s.builtin && (
                            <Badge variant="secondary" className="px-1.5 py-0 text-[10px]">
                              {t("agent.scenarioBuiltin")}
                            </Badge>
                          )}
                        </p>
                        <p className="mt-0.5 line-clamp-2 text-[11px] text-muted-foreground">
                          {s.description}
                        </p>
                      </div>
                      <div className="flex shrink-0 items-center gap-0.5 opacity-0 transition-opacity focus-within:opacity-100 group-hover:opacity-100">
                        <button
                          type="button"
                          aria-label={t("common.edit")}
                          title={t("common.edit")}
                          onClick={async () => {
                            try {
                              const full = await agentApi.getSkill(s.name);
                              setEditor({
                                original: s.name,
                                name: s.name,
                                content: full.content
                                  ? `---\nname: ${full.name}\ndescription: ${full.description}\n---\n\n${full.content}`
                                  : (full.content ?? ""),
                              });
                            } catch (e) {
                              toast.error(e instanceof Error ? e.message : String(e));
                            }
                          }}
                          className="rounded p-1 text-muted-foreground hover:bg-accent hover:text-foreground"
                        >
                          <Pencil className="h-3.5 w-3.5" />
                        </button>
                        <button
                          type="button"
                          aria-label={t("common.delete")}
                          title={t("common.delete")}
                          onClick={() => void handleDelete(s)}
                          className="rounded p-1 text-muted-foreground hover:bg-accent hover:text-destructive"
                        >
                          <Trash2 className="h-3.5 w-3.5" />
                        </button>
                      </div>
                    </li>
                  ))}
                </ul>
              )}
            </div>
            <DialogFooter>
              <Button
                type="button"
                size="sm"
                onClick={() => setEditor({ original: "", name: "", content: "" })}
              >
                <Plus className="h-3.5 w-3.5" />
                {t("agent.scenarioNew")}
              </Button>
            </DialogFooter>
          </>
        )}
      </DialogContent>
    </Dialog>
  );
}

/**
 * SaveScenarioDialog — 沉淀闭环: distills a persisted conversation into a
 * SKILL.md draft via a one-shot LLM call, shows an editable preview (name +
 * full content), and writes it into the skills directory on confirm.
 */
export function SaveScenarioDialog({
  conv,
  providerID,
  model,
  open,
  onOpenChange,
}: {
  conv: ConversationDTO | null;
  providerID: string;
  model: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const [name, setName] = useState("");
  const [content, setContent] = useState("");
  const [generating, setGenerating] = useState(false);
  const [saving, setSaving] = useState(false);

  // Generate the draft when the dialog opens for a conversation.
  useEffect(() => {
    if (!open || !conv) return;
    let cancelled = false;
    setName("");
    setContent("");
    setGenerating(true);
    agentApi
      .draftScenario(conv.id, providerID, model)
      .then((d) => {
        if (cancelled) return;
        setName(d.name || "");
        setContent(d.content);
      })
      .catch((e) => {
        if (!cancelled) toast.error(e instanceof Error ? e.message : String(e));
        onOpenChange(false);
      })
      .finally(() => {
        if (!cancelled) setGenerating(false);
      });
    return () => {
      cancelled = true;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, conv?.id]);

  const handleSave = async () => {
    if (!NAME_RE.test(name)) {
      toast.error(t("agent.scenarioNameInvalid"));
      return;
    }
    setSaving(true);
    try {
      await agentApi.saveSkill(name, content);
      toast.info(t("agent.scenarioSaved"));
      queryClient.invalidateQueries({ queryKey: AGENT_SKILLS_KEY }).catch(() => {});
      onOpenChange(false);
    } catch (e) {
      toast.error(e instanceof Error ? e.message : String(e));
    } finally {
      setSaving(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-xl">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <Sparkles className="h-4 w-4 text-primary" />
            {t("agent.saveScenarioTitle")}
          </DialogTitle>
          <DialogDescription>
            {conv?.title || conv?.hostName || ""} — {t("agent.saveScenarioHint")}
          </DialogDescription>
        </DialogHeader>

        {generating ? (
          <div className="flex items-center justify-center gap-2 py-10 text-sm text-muted-foreground">
            <Loader2 className="h-4 w-4 animate-spin" />
            {t("agent.saveScenarioGenerating")}
          </div>
        ) : (
          <div className="flex flex-col gap-3">
            <div className="flex items-center gap-2">
              <span className="shrink-0 font-mono text-xs text-muted-foreground">/</span>
              <Input
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="redis-conn-refused"
                aria-label={t("agent.scenarioName")}
                className="h-8 max-w-56 font-mono text-xs"
              />
            </div>
            <textarea
              value={content}
              onChange={(e) => setContent(e.target.value)}
              rows={14}
              spellCheck={false}
              aria-label={t("agent.scenarioContent")}
              className="w-full resize-y rounded-[var(--radius)] border border-input bg-background p-3 font-mono text-xs leading-relaxed focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
            />
          </div>
        )}

        <DialogFooter>
          <Button
            type="button"
            variant="ghost"
            size="sm"
            onClick={() => onOpenChange(false)}
            disabled={saving}
          >
            {t("common.cancel")}
          </Button>
          <Button
            type="button"
            size="sm"
            onClick={() => void handleSave()}
            disabled={generating || saving || !name.trim() || !content.trim()}
          >
            <Save className="h-3.5 w-3.5" />
            {t("agent.saveScenarioConfirm")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
