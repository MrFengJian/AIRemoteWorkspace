import { useEffect } from "react";
import { Events } from "@wailsio/runtime";
import { useTranslation } from "react-i18next";
import { ShieldAlert } from "lucide-react";

import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { useApprovalStore, type PendingApproval } from "@/features/agent/approval.store";
import { useTerminalStore } from "@/features/terminal/terminal.store";
import { agentApi } from "@/features/agent/api";
import { cn } from "@/lib/utils";

/**
 * Single global host for agent tool approvals. Mounted once in AppShell so
 * approval dialogs appear no matter which view/panel is active — a WRITE or
 * DANGEROUS tool would otherwise block silently until its 5-minute timeout.
 *
 * Deliberately strict: ESC, clicking the overlay, and the close button do
 * NOT deny — denying is an explicit decision, so only the Deny button does
 * it. The dialog shows which host the operation targets.
 */
export function ApprovalHost() {
  const queue = useApprovalStore((s) => s.queue);
  const add = useApprovalStore((s) => s.add);
  const remove = useApprovalStore((s) => s.remove);

  const sessions = useTerminalStore((s) => s.sessions);

  // Approvals arrive on a single global event (not per-session).
  useEffect(() => {
    const cancel = Events.On("agent:approval", (e: unknown) => {
      const data = (e as { data?: unknown }).data as
        | { reqId?: string; sessionId?: string; toolName?: string; permission?: string; args?: string }
        | undefined;
      if (data?.reqId) {
        add({
          reqId: data.reqId,
          sessionId: data.sessionId ?? "",
          toolName: data.toolName ?? "",
          permission: data.permission ?? "",
          args: data.args ?? "",
        });
      }
    });
    return () => {
      if (typeof cancel === "function") cancel();
    };
  }, [add]);

  const current = queue[0];
  if (!current) return null;

  // Resolve the target host from the terminal session (multi-host safety).
  const hostName = sessions.find((s) => s.id === current.sessionId)?.hostName;

  const resolve = (approved: boolean) => {
    agentApi.approveToolCall(current.reqId, approved).catch(() => {});
    remove(current.reqId);
  };

  return (
    <Dialog open>
      <DialogContent
        className="max-w-md"
        // Denying is an explicit decision — ESC/overlay must not silently deny
        // a dangerous operation. The built-in close button is inert too: this
        // Dialog's open state is fully controlled and never unset here.
        onEscapeKeyDown={(e) => e.preventDefault()}
        onInteractOutside={(e) => e.preventDefault()}
      >
        <ApprovalBody approval={current} hostName={hostName} queueLength={queue.length} onResolve={resolve} />
      </DialogContent>
    </Dialog>
  );
}

function ApprovalBody({
  approval,
  hostName,
  queueLength,
  onResolve,
}: {
  approval: PendingApproval;
  hostName?: string;
  queueLength: number;
  onResolve: (approved: boolean) => void;
}) {
  const { t } = useTranslation();
  const isDangerous = approval.permission === "dangerous";

  // Parse the args JSON to extract the command/path for prominent display.
  let commandText = approval.args;
  let argLabel = t("agent.approvalArgs");
  try {
    const parsed = JSON.parse(approval.args);
    if (parsed.command) {
      commandText = parsed.command;
      argLabel = t("agent.approvalCommand");
    } else if (parsed.path) {
      commandText = parsed.path;
      argLabel = t("agent.approvalPath");
    } else if (parsed.remotePath) {
      commandText = `${parsed.remotePath}${parsed.localPath ? " ← " + parsed.localPath : ""}`;
      argLabel = t("agent.approvalFile");
    }
  } catch {
    // not JSON; show raw
  }

  return (
    <>
      <DialogHeader>
        <DialogTitle className="flex flex-wrap items-center gap-2 text-base">
          <ShieldAlert className={cn("h-5 w-5", isDangerous ? "text-destructive" : "text-warning")} />
          {isDangerous ? t("agent.approvalDangerous") : t("agent.approvalTitle")}
          {hostName && (
            <Badge variant="outline" className="font-mono text-xs">
              {hostName}
            </Badge>
          )}
          {queueLength > 1 && (
            <Badge variant="secondary" className="text-[10px]">
              {t("agent.pendingApprovals", { count: queueLength - 1 })}
            </Badge>
          )}
        </DialogTitle>
        <DialogDescription>
          {t("agent.approvalDesc", { permission: approval.permission })}
        </DialogDescription>
      </DialogHeader>

      {/* The command/operation — displayed prominently */}
      <div
        className={cn(
          "rounded-[var(--radius)] border p-3",
          isDangerous
            ? "border-destructive/40 bg-destructive/10"
            : "border-warning/40 bg-warning/10",
        )}
      >
        <div className="mb-1.5 flex items-center gap-2">
          <span className="font-mono text-xs font-semibold text-foreground">
            {approval.toolName}
          </span>
          <span className="text-[10px] uppercase tracking-wide text-muted-foreground">
            {argLabel}
          </span>
        </div>
        <pre className="max-h-40 overflow-auto whitespace-pre-wrap break-all rounded bg-background/60 p-2 font-mono text-xs text-foreground">
          {commandText}
        </pre>
      </div>

      {isDangerous && (
        <p className="rounded-[var(--radius)] border border-destructive/30 bg-destructive/5 p-2 text-xs text-destructive">
          {t("agent.approvalDanger")}
        </p>
      )}

      <DialogFooter>
        <Button
          variant="outline"
          onClick={() => onResolve(false)}
          aria-label={t("agent.deny")}
        >
          {t("agent.deny")}
        </Button>
        <Button
          variant={isDangerous ? "destructive" : "default"}
          className={cn(!isDangerous && "bg-primary text-primary-foreground")}
          onClick={() => onResolve(true)}
          aria-label={t("agent.approve")}
        >
          {t("agent.approve")}
        </Button>
      </DialogFooter>
    </>
  );
}
