import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { useQuery } from "@tanstack/react-query";
import { Plus, X } from "lucide-react";

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
import { Label } from "@/components/ui/label";
import { Select } from "@/components/ui/select";
import { Checkbox } from "@/components/ui/checkbox";
import { Badge } from "@/components/ui/badge";
import { agentApi } from "@/features/agent/api";
import { useModelProviders } from "@/features/settings/hooks";
import { expertsApi, type ExpertDTO } from "@/features/experts/api";
import { EXPERT_COLOR_GRADIENTS, EXPERT_COLOR_IDS, EXPERT_ICON_IDS, ExpertAvatar } from "@/features/experts/avatar";
import { cn } from "@/lib/utils";
import { toast, errorMessage } from "@/lib/toast";

/** Tools an expert allowlist can scope. The `skill` tool is deliberately
 *  absent — it is the persona's knowledge channel and always available. */
const TOOL_IDS = [
  { id: "ssh_exec", key: "sshExec" },
  { id: "ssh_read_file", key: "sshReadFile" },
  { id: "ssh_write_file", key: "sshWriteFile" },
  { id: "upload", key: "upload" },
  { id: "download", key: "download" },
  { id: "local_exec", key: "localExec" },
  { id: "local_read_file", key: "localReadFile" },
] as const;

function blankExpert(): ExpertDTO {
  return {
    id: "",
    name: "",
    role: "",
    icon: "Bot",
    color: "blue",
    description: "",
    systemPrompt: "",
    allowedTools: [],
    skillRefs: [],
    providerId: "",
    model: "",
    policy: "",
    temperature: 0,
    maxSteps: 0,
    openingMessage: "",
    suggestedPrompts: [],
    autoSnapshot: false,
    builtin: false,
    enabled: true,
    sortOrder: 100,
  };
}

interface ExpertFormDialogProps {
  open: boolean;
  /** null = create blank; an expert with id "" (duplicate) presets the form. */
  expert: ExpertDTO | null;
  onClose: () => void;
  onSaved: () => void;
}

/**
 * ExpertFormDialog — create/edit a digital employee. Sections follow the
 * persona-card convention: identity, persona prompt, capabilities (tool
 * allowlist + bound skills), model binding, interaction design.
 */
export function ExpertFormDialog({ open, expert, onClose, onSaved }: ExpertFormDialogProps) {
  const { t } = useTranslation();
  const [form, setForm] = useState<ExpertDTO>(blankExpert());
  const [saving, setSaving] = useState(false);

  const { data: providers } = useModelProviders();
  const { data: skills } = useQuery({
    queryKey: ["agent-skills"],
    queryFn: agentApi.listSkills,
    enabled: open,
  });

  // Sync the form state each time the dialog opens (add/edit/duplicate).
  useEffect(() => {
    if (open) {
      setForm(expert ? { ...blankExpert(), ...expert } : blankExpert());
    }
  }, [open, expert]);

  const patch = (p: Partial<ExpertDTO>) => setForm((f) => ({ ...f, ...p }));

  // Empty allowlist = every tool; the master checkbox drives that mode.
  const allTools = (form.allowedTools?.length ?? 0) === 0;
  const toggleAllTools = (v: boolean) => patch({ allowedTools: v ? [] : TOOL_IDS.map((x) => x.id) });
  const toggleTool = (id: string, on: boolean) => {
    const cur = new Set(form.allowedTools ?? []);
    if (on) cur.add(id);
    else cur.delete(id);
    patch({ allowedTools: [...cur] });
  };
  const toggleSkill = (name: string, on: boolean) => {
    const cur = new Set(form.skillRefs ?? []);
    if (on) cur.add(name);
    else cur.delete(name);
    patch({ skillRefs: [...cur] });
  };

  const setPrompt = (i: number, text: string) => {
    patch({ suggestedPrompts: (form.suggestedPrompts ?? []).map((p, j) => (j === i ? text : p)) });
  };
  const addPrompt = () => patch({ suggestedPrompts: [...(form.suggestedPrompts ?? []), ""] });
  const removePrompt = (i: number) =>
    patch({ suggestedPrompts: (form.suggestedPrompts ?? []).filter((_, j) => j !== i) });

  const save = async () => {
    if (!form.name.trim()) {
      toast.error(t("settings.experts.nameRequired"));
      return;
    }
    setSaving(true);
    try {
      await expertsApi.save({
        ...form,
        temperature: Math.min(2, Math.max(0, Number(form.temperature) || 0)),
        maxSteps: Math.max(0, Math.round(Number(form.maxSteps) || 0)),
      });
      toast.success(t("settings.experts.saved"));
      onSaved();
      onClose();
    } catch (e) {
      toast.error(`${t("settings.experts.saveFailed")}: ${errorMessage(e)}`);
    } finally {
      setSaving(false);
    }
  };

  const selectedProvider = providers?.find((p) => p.id === form.providerId);

  return (
    <Dialog open={open} onOpenChange={(v) => !v && onClose()}>
      <DialogContent className="max-h-[85vh] max-w-2xl overflow-auto">
        <DialogHeader>
          <DialogTitle>
            {form.id ? t("settings.experts.editTitle") : t("settings.experts.addTitle")}
          </DialogTitle>
          <DialogDescription>{t("settings.experts.formDesc")}</DialogDescription>
        </DialogHeader>

        <div className="flex flex-col gap-5">
          {/* ── Identity ─────────────────────────────────────────────── */}
          <section className="flex flex-col gap-3">
            <h3 className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
              {t("settings.experts.sectionIdentity")}
            </h3>
            <div className="grid grid-cols-[6rem_1fr] items-center gap-3">
              <Label htmlFor="expert-name">{t("settings.experts.name")}</Label>
              <Input id="expert-name" value={form.name} onChange={(e) => patch({ name: e.target.value })} />
            </div>
            <div className="grid grid-cols-[6rem_1fr] items-center gap-3">
              <Label htmlFor="expert-role">{t("settings.experts.role")}</Label>
              <Input id="expert-role" value={form.role} onChange={(e) => patch({ role: e.target.value })} placeholder={t("settings.experts.rolePlaceholder")} />
            </div>
            <div className="grid grid-cols-[6rem_1fr] items-center gap-3">
              <Label htmlFor="expert-desc">{t("settings.experts.desc")}</Label>
              <Input id="expert-desc" value={form.description} onChange={(e) => patch({ description: e.target.value })} />
            </div>
            <div className="grid grid-cols-[6rem_1fr] items-center gap-3">
              <Label>{t("settings.experts.avatar")}</Label>
              <div className="flex flex-wrap items-center gap-2">
                <div className="flex gap-1">
                  {EXPERT_ICON_IDS.map((icon) => (
                    <button
                      key={icon}
                      type="button"
                      onClick={() => patch({ icon })}
                      title={icon}
                      className={cn(
                        "flex h-8 w-8 items-center justify-center rounded-[var(--radius)] border transition-colors",
                        form.icon === icon
                          ? "border-primary bg-primary/10 text-primary"
                          : "border-border text-muted-foreground hover:bg-accent",
                      )}
                    >
                      <ExpertAvatar icon={icon} color={form.color} className="h-5 w-5" iconClassName="h-3 w-3" />
                    </button>
                  ))}
                </div>
                <div className="flex gap-1">
                  {EXPERT_COLOR_IDS.map((color) => (
                    <button
                      key={color}
                      type="button"
                      onClick={() => patch({ color })}
                      title={color}
                      className={cn(
                        "h-6 w-6 rounded-full bg-gradient-to-br shadow-sm ring-1 transition-transform",
                        EXPERT_COLOR_GRADIENTS[color],
                        form.color === color ? "scale-110 ring-2 ring-primary" : "ring-border",
                      )}
                    />
                  ))}
                </div>
                <ExpertAvatar icon={form.icon} color={form.color} className="h-8 w-8" iconClassName="h-4 w-4" />
              </div>
            </div>
          </section>

          {/* ── Persona ──────────────────────────────────────────────── */}
          <section className="flex flex-col gap-2">
            <h3 className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
              {t("settings.experts.sectionPersona")}
            </h3>
            <textarea
              rows={8}
              value={form.systemPrompt}
              onChange={(e) => patch({ systemPrompt: e.target.value })}
              placeholder={t("settings.experts.promptPlaceholder")}
              className="w-full resize-y rounded-[var(--radius)] border border-input bg-background px-3 py-2 font-mono text-xs focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
            />
            <p className="text-[11px] leading-relaxed text-muted-foreground">
              {t("settings.experts.promptHint")}
            </p>
          </section>

          {/* ── Capabilities ─────────────────────────────────────────── */}
          <section className="flex flex-col gap-3">
            <h3 className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
              {t("settings.experts.sectionCapabilities")}
            </h3>
            <label className="flex items-center gap-2 text-sm">
              <Checkbox checked={allTools} onCheckedChange={(v) => toggleAllTools(v === true)} />
              <span>{t("settings.experts.allTools")}</span>
            </label>
            {!allTools && (
              <div className="grid grid-cols-2 gap-x-4 gap-y-1.5 rounded-[var(--radius)] border border-border p-3">
                {TOOL_IDS.map((tool) => (
                  <label key={tool.id} className="flex items-center gap-2 text-xs">
                    <Checkbox
                      checked={(form.allowedTools ?? []).includes(tool.id)}
                      onCheckedChange={(v) => toggleTool(tool.id, v === true)}
                    />
                    <span className="font-mono">{tool.id}</span>
                  </label>
                ))}
              </div>
            )}
            <div>
              <Label className="text-xs">{t("settings.experts.boundSkills")}</Label>
              {(skills ?? []).length === 0 ? (
                <p className="mt-1 text-[11px] text-muted-foreground">{t("settings.experts.noSkills")}</p>
              ) : (
                <div className="mt-1.5 grid grid-cols-2 gap-x-4 gap-y-1.5">
                  {(skills ?? []).map((s) => (
                    <label key={s.name} className="flex items-center gap-2 text-xs" title={s.description}>
                      <Checkbox
                        checked={(form.skillRefs ?? []).includes(s.name)}
                        onCheckedChange={(v) => toggleSkill(s.name, v === true)}
                      />
                      <span className="truncate font-mono">{s.name}</span>
                    </label>
                  ))}
                </div>
              )}
            </div>
          </section>

          {/* ── Model binding ────────────────────────────────────────── */}
          <section className="flex flex-col gap-3">
            <h3 className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
              {t("settings.experts.sectionModel")}
            </h3>
            <div className="grid grid-cols-[10rem_1fr] items-center gap-3">
              <Label htmlFor="expert-provider">{t("settings.experts.defaultModel")}</Label>
              <div className="flex gap-2">
                <Select
                  id="expert-provider"
                  value={form.providerId}
                  onChange={(e) => patch({ providerId: e.target.value, model: "" })}
                  className="h-8 max-w-[50%] text-xs"
                >
                  <option value="">{t("settings.experts.followSession")}</option>
                  {(providers ?? []).filter((p) => p.enabled).map((p) => (
                    <option key={p.id} value={p.id}>{p.name}</option>
                  ))}
                </Select>
                {selectedProvider && (selectedProvider.models?.length ?? 0) > 0 && (
                  <Select
                    value={form.model}
                    onChange={(e) => patch({ model: e.target.value })}
                    className="h-8 min-w-0 flex-1 font-mono text-xs"
                  >
                    {selectedProvider.models!.map((m) => (
                      <option key={m} value={m}>{m}</option>
                    ))}
                  </Select>
                )}
              </div>
            </div>
            <div className="grid grid-cols-[10rem_1fr] items-center gap-3">
              <Label htmlFor="expert-policy">{t("settings.experts.defaultPolicy")}</Label>
              <Select
                id="expert-policy"
                value={form.policy}
                onChange={(e) => patch({ policy: e.target.value })}
                className="h-8 max-w-56 text-xs"
              >
                <option value="">{t("settings.experts.policyFollow")}</option>
                <option value="strict">{t("agent.policyStrict")}</option>
                <option value="auto_write">{t("agent.policyAutoWrite")}</option>
              </Select>
            </div>
            <div className="grid grid-cols-[10rem_1fr] items-center gap-3">
              <Label htmlFor="expert-temp">{t("settings.experts.temperature")}</Label>
              <div className="flex items-center gap-3">
                <Input
                  id="expert-temp"
                  type="number"
                  min={0}
                  max={2}
                  step={0.1}
                  value={form.temperature || 0}
                  onChange={(e) => patch({ temperature: Number(e.target.value) })}
                  className="h-8 max-w-24"
                />
                <span className="text-[11px] text-muted-foreground">{t("settings.experts.temperatureHint")}</span>
              </div>
            </div>
            <div className="grid grid-cols-[10rem_1fr] items-center gap-3">
              <Label htmlFor="expert-steps">{t("settings.experts.maxSteps")}</Label>
              <div className="flex items-center gap-3">
                <Input
                  id="expert-steps"
                  type="number"
                  min={0}
                  step={10}
                  value={form.maxSteps || 0}
                  onChange={(e) => patch({ maxSteps: Number(e.target.value) })}
                  className="h-8 max-w-24"
                />
                <span className="text-[11px] text-muted-foreground">{t("settings.experts.maxStepsHint")}</span>
              </div>
            </div>
          </section>

          {/* ── Interaction ──────────────────────────────────────────── */}
          <section className="flex flex-col gap-3">
            <h3 className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
              {t("settings.experts.sectionInteraction")}
            </h3>
            <div className="grid grid-cols-[10rem_1fr] items-start gap-3">
              <Label htmlFor="expert-opening">{t("settings.experts.opening")}</Label>
              <textarea
                id="expert-opening"
                rows={3}
                value={form.openingMessage}
                onChange={(e) => patch({ openingMessage: e.target.value })}
                placeholder={t("settings.experts.openingPlaceholder")}
                className="w-full resize-y rounded-[var(--radius)] border border-input bg-background px-3 py-2 text-sm focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
              />
            </div>
            <div className="grid grid-cols-[10rem_1fr] items-start gap-3">
              <Label>{t("settings.experts.suggested")}</Label>
              <div className="flex flex-col gap-1.5">
                {(form.suggestedPrompts ?? []).length === 0 && (
                  <p className="text-[11px] text-muted-foreground">{t("settings.experts.noSuggested")}</p>
                )}
                {(form.suggestedPrompts ?? []).map((p, i) => (
                  <div key={i} className="flex items-center gap-2">
                    <Input
                      value={p}
                      onChange={(e) => setPrompt(i, e.target.value)}
                      className="h-8 flex-1 text-xs"
                    />
                    <Button
                      variant="ghost"
                      size="icon"
                      className="h-7 w-7 shrink-0 text-muted-foreground hover:text-destructive"
                      onClick={() => removePrompt(i)}
                      aria-label={t("common.delete")}
                    >
                      <X className="h-3.5 w-3.5" />
                    </Button>
                  </div>
                ))}
                <Button variant="outline" size="sm" className="h-7 w-fit px-2 text-xs" onClick={addPrompt}>
                  <Plus className="h-3 w-3" />
                  {t("settings.experts.addSuggested")}
                </Button>
              </div>
            </div>
          </section>

          {form.builtin && (
            <div className="flex items-center gap-2">
              <Badge variant="secondary">{t("settings.experts.builtin")}</Badge>
              <span className="text-[11px] text-muted-foreground">{t("settings.experts.builtinHint")}</span>
            </div>
          )}
        </div>

        <DialogFooter>
          <Button type="button" variant="ghost" size="sm" onClick={onClose}>
            {t("common.cancel")}
          </Button>
          <Button type="button" size="sm" onClick={() => void save()} disabled={saving || !form.name.trim()}>
            {t("common.save")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
