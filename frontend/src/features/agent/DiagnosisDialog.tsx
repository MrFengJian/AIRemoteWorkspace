import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Stethoscope } from "lucide-react";

import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import type { SkillDTO } from "@/features/agent/api";

/**
 * DiagnosisDialog — the Agent panel's diagnosis entry (DIAGNOSIS_AGENT.md
 * Phase A). The user describes the symptom (optionally seeded from a builtin
 * scenario chip, which prefixes the message with `/name` so the runtime
 * inlines that playbook); on start the backend auto-collects the deterministic
 * health snapshot and opens the diagnosis-mode conversation.
 */
interface DiagnosisDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  builtinScenarios: SkillDTO[];
  onStart: (symptom: string) => void;
}

export function DiagnosisDialog({ open, onOpenChange, builtinScenarios, onStart }: DiagnosisDialogProps) {
  const { t } = useTranslation();
  const [symptom, setSymptom] = useState("");
  const textareaRef = useRef<HTMLTextAreaElement | null>(null);

  useEffect(() => {
    if (open) {
      setSymptom("");
      // Focus once the dialog content mounts (radix portals async).
      requestAnimationFrame(() => textareaRef.current?.focus());
    }
  }, [open]);

  const start = () => {
    const text = symptom.trim();
    if (!text) return;
    onOpenChange(false);
    onStart(text);
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <Stethoscope className="h-4 w-4 text-primary" />
            {t("agent.diagnoseTitle")}
          </DialogTitle>
          <DialogDescription>{t("agent.diagnoseHint")}</DialogDescription>
        </DialogHeader>

        {builtinScenarios.length > 0 && (
          <div className="flex flex-wrap gap-1.5">
            {builtinScenarios.map((s) => (
              <button
                key={s.name}
                type="button"
                title={s.description}
                onClick={() => {
                  setSymptom((cur) => (cur.trim() ? cur : `/${s.name} `));
                  requestAnimationFrame(() => textareaRef.current?.focus());
                }}
                className="rounded-full border border-border px-2.5 py-1 text-xs text-muted-foreground transition-colors hover:border-primary/50 hover:bg-accent hover:text-foreground"
              >
                {s.name}
              </button>
            ))}
          </div>
        )}

        <textarea
          ref={textareaRef}
          rows={4}
          value={symptom}
          onChange={(e) => setSymptom(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter" && (e.ctrlKey || e.metaKey)) {
              e.preventDefault();
              start();
            }
          }}
          placeholder={t("agent.diagnosePlaceholder")}
          aria-label={t("agent.diagnoseTitle")}
          className="w-full resize-none rounded-[var(--radius)] border border-input bg-background px-3 py-2 text-sm focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
        />

        <p className="text-[11px] leading-relaxed text-muted-foreground">
          {t("agent.diagnoseSnapshotNote")}
        </p>

        <DialogFooter>
          <Button type="button" variant="ghost" size="sm" onClick={() => onOpenChange(false)}>
            {t("common.cancel")}
          </Button>
          <Button type="button" size="sm" onClick={start} disabled={!symptom.trim()}>
            <Stethoscope className="h-3.5 w-3.5" />
            {t("agent.diagnoseStart")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
