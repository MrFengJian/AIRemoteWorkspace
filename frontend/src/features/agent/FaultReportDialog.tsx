import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { FileWarning, Loader2, Save } from "lucide-react";

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
import { AgentMarkdown } from "@/features/agent/AgentMarkdown";
import { agentApi, type ConversationDTO } from "@/features/agent/api";
import { FAULT_SEVERITIES, faultsApi } from "@/features/faults/api";
import { toast, errorMessage } from "@/lib/toast";

interface FaultReportDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** The session's active conversation (transcript source); null = none. */
  conversation: ConversationDTO | null;
  providerID: string;
  model: string;
  onSaved: () => void;
}

/**
 * FaultReportDialog — 故障报告沉淀: distills the active conversation into a
 * structured incident report via a one-shot LLM call (SRE diagnosis report
 * format), shows an editable preview (title / severity / body) and saves it
 * as a host-attached asset on the 故障报告 page.
 */
export function FaultReportDialog({
  open,
  onOpenChange,
  conversation,
  providerID,
  model,
  onSaved,
}: FaultReportDialogProps) {
  const { t } = useTranslation();
  const [title, setTitle] = useState("");
  const [severity, setSeverity] = useState("warning");
  const [body, setBody] = useState("");
  const [generating, setGenerating] = useState(false);
  const [saving, setSaving] = useState(false);

  // Generate the draft when the dialog opens for a conversation.
  useEffect(() => {
    if (!open || !conversation) return;
    let cancelled = false;
    setTitle("");
    setBody("");
    setGenerating(true);
    agentApi
      .draftFaultReport(conversation.id, providerID, model)
      .then((d) => {
        if (cancelled) return;
        setTitle(d.title);
        setSeverity(d.severity || "warning");
        setBody(d.body);
      })
      .catch((e) => {
        if (!cancelled) toast.error(errorMessage(e));
        onOpenChange(false);
      })
      .finally(() => {
        if (!cancelled) setGenerating(false);
      });
    return () => {
      cancelled = true;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, conversation?.id]);

  const handleSave = async () => {
    if (!conversation) return;
    if (!title.trim() || !body.trim()) {
      toast.error(t("agent.reportIncomplete"));
      return;
    }
    setSaving(true);
    try {
      await faultsApi.save({
        id: "",
        hostId: conversation.hostId,
        hostName: conversation.hostName,
        title: title.trim(),
        severity,
        status: "open",
        body: body.trim(),
        conversationId: conversation.id,
        createdAt: "",
        updatedAt: "",
      });
      toast.info(t("agent.reportSaved"));
      onSaved();
      onOpenChange(false);
    } catch (e) {
      toast.error(e instanceof Error ? e.message : String(e));
    } finally {
      setSaving(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="flex max-h-[85vh] max-w-2xl flex-col overflow-auto">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <FileWarning className="h-4 w-4 text-primary" />
            {t("agent.reportTitle")}
          </DialogTitle>
          <DialogDescription>
            {conversation
              ? `${conversation.hostName || t("faults.local")} — ${t("agent.reportHint")}`
              : t("agent.reportNoConversation")}
          </DialogDescription>
        </DialogHeader>

        {generating ? (
          <div className="flex items-center justify-center gap-2 py-10 text-sm text-muted-foreground">
            <Loader2 className="h-4 w-4 animate-spin" />
            {t("agent.reportGenerating")}
          </div>
        ) : (
          <div className="flex flex-col gap-3">
            <div className="grid grid-cols-[1fr_10rem] gap-2">
              <div className="grid gap-1.5">
                <Label htmlFor="report-title">{t("faults.reportTitleLabel")}</Label>
                <Input
                  id="report-title"
                  value={title}
                  onChange={(e) => setTitle(e.target.value)}
                  placeholder={t("agent.reportTitlePlaceholder")}
                />
              </div>
              <div className="grid gap-1.5">
                <Label htmlFor="report-severity">{t("faults.severity")}</Label>
                <Select
                  id="report-severity"
                  value={severity}
                  onChange={(e) => setSeverity(e.target.value)}
                >
                  {FAULT_SEVERITIES.map((s) => (
                    <option key={s} value={s}>{t(`faults.severity_${s}`)}</option>
                  ))}
                </Select>
              </div>
            </div>
            <Label htmlFor="report-body">{t("faults.reportBodyLabel")}</Label>
            <textarea
              id="report-body"
              value={body}
              onChange={(e) => setBody(e.target.value)}
              rows={12}
              spellCheck={false}
              aria-label={t("faults.reportBodyLabel")}
              className="w-full resize-y rounded-[var(--radius)] border border-input bg-background p-3 font-mono text-xs leading-relaxed focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
            />
            {body.trim().length > 0 && (
              <details className="rounded-[var(--radius)] border border-border px-3 py-2 text-xs">
                <summary className="cursor-pointer text-muted-foreground">
                  {t("agent.reportPreview")}
                </summary>
                <div className="mt-2 text-sm">
                  <AgentMarkdown content={body} canInsert={false} onInsert={() => {}} />
                </div>
              </details>
            )}
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
            disabled={generating || saving || !title.trim() || !body.trim()}
          >
            <Save className="h-3.5 w-3.5" />
            {t("agent.reportSave")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
