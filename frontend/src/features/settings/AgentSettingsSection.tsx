import { useTranslation } from "react-i18next";
import { RotateCcw } from "lucide-react";

import type { AppConfig, AgentConfig } from "@/../bindings/github.com/ai-remote/workspace/internal/domain";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

const DEFAULTS: AgentConfig = {
  maxSteps: 100,
  historyTurns: 40,
  toolOutputLimitKB: 64,
  customInstructions: "",
};

/**
 * Agent runtime settings (global): ReAct step budget, conversation memory
 * depth, tool-output cap, and standing instructions appended to the built-in
 * system prompt. Changes apply to the next chat turn — no restart needed.
 */
export function AgentSettingsSection({
  config,
  update,
}: {
  config: AppConfig;
  update: (patch: Partial<AppConfig>) => Promise<boolean>;
}) {
  const { t } = useTranslation();
  const agent = { ...DEFAULTS, ...config.agent };

  /** Update one numeric field with clamping; roll back on failure. */
  const setNum = (key: "maxSteps" | "historyTurns" | "toolOutputLimitKB", raw: string, min: number, max: number) => {
    const n = Math.round(Number(raw));
    if (!Number.isFinite(n)) return;
    void update({ agent: { ...agent, [key]: Math.min(max, Math.max(min, n)) } });
  };

  return (
    <div className="flex flex-col gap-4">
      <h2 className="text-lg font-semibold">{t("settings.agentTitle")}</h2>

      <Card>
        <CardHeader>
          <CardTitle className="text-sm">{t("settings.agentRuntime")}</CardTitle>
          <CardDescription>{t("settings.agentRuntimeDesc")}</CardDescription>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          <div className="grid grid-cols-[10rem_9rem_1fr] items-center gap-3">
            <Label htmlFor="agentMaxSteps">{t("settings.agentMaxSteps")}</Label>
            <Input
              id="agentMaxSteps"
              type="number"
              min={5}
              max={1000}
              value={agent.maxSteps}
              onChange={(e) => setNum("maxSteps", e.target.value, 5, 1000)}
            />
            <p className="text-xs text-muted-foreground">{t("settings.agentMaxStepsHint")}</p>
          </div>
          <div className="grid grid-cols-[10rem_9rem_1fr] items-center gap-3">
            <Label htmlFor="agentHistoryTurns">{t("settings.agentHistoryTurns")}</Label>
            <Input
              id="agentHistoryTurns"
              type="number"
              min={2}
              max={200}
              value={agent.historyTurns}
              onChange={(e) => setNum("historyTurns", e.target.value, 2, 200)}
            />
            <p className="text-xs text-muted-foreground">{t("settings.agentHistoryTurnsHint")}</p>
          </div>
          <div className="grid grid-cols-[10rem_9rem_1fr] items-center gap-3">
            <Label htmlFor="agentOutputLimit">{t("settings.agentOutputLimit")}</Label>
            <Input
              id="agentOutputLimit"
              type="number"
              min={8}
              max={2048}
              value={agent.toolOutputLimitKB}
              onChange={(e) => setNum("toolOutputLimitKB", e.target.value, 8, 2048)}
            />
            <p className="text-xs text-muted-foreground">{t("settings.agentOutputLimitHint")}</p>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <div className="flex items-center justify-between">
            <CardTitle className="text-sm">{t("settings.agentPrompt")}</CardTitle>
            {config.agent?.customInstructions ? (
              <Button
                variant="ghost"
                size="sm"
                className="h-7 gap-1 px-2 text-xs text-muted-foreground"
                onClick={() => void update({ agent: { ...agent, customInstructions: "" } })}
              >
                <RotateCcw className="h-3 w-3" />
                {t("settings.agentPromptReset")}
              </Button>
            ) : null}
          </div>
          <CardDescription>{t("settings.agentPromptDesc")}</CardDescription>
        </CardHeader>
        <CardContent>
          <textarea
            value={agent.customInstructions}
            onChange={(e) => void update({ agent: { ...agent, customInstructions: e.target.value } })}
            placeholder={t("settings.agentPromptPlaceholder")}
            rows={8}
            className="w-full resize-y rounded-[var(--radius)] border border-border bg-background px-3 py-2 font-mono text-xs text-foreground placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
          />
        </CardContent>
      </Card>
    </div>
  );
}
