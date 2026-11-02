import { useEffect, useRef, useState } from "react";
import type * as React from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { Events } from "@wailsio/runtime";
import {
  AlertTriangle,
  ArrowLeft,
  CheckCircle2,
  ChevronRight,
  Loader2,
  Package,
  Play,
  Rocket,
  Settings2,
  XCircle,
} from "lucide-react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useUIStore } from "@/stores/ui.store";
import { isLocalSession, useTerminalStore } from "@/features/terminal/terminal.store";
import { toast, errorMessage } from "@/lib/toast";
import { cn } from "@/lib/utils";
import {
  appsApi,
  type CatalogEntry,
  type DeployStepRun,
  type DeployTask,
  type DeployTaskEvent,
} from "./api";
import { AppManagePanel } from "./AppManagePanel";

/**
 * 会话右侧"应用"面板：当前主机可安装的应用（内置目录）+ 已安装应用（状态
 * 管理）。安装向导（参数 → 直接执行，过程只看实时输出）与管理面板都在这
 * 里，所有操作经当前会话 SSH 通道执行。一级"应用"页选主机后跳转到这里。
 */
export function AppsPanel(props: { embeddedSessionID: string }) {
  const { t } = useTranslation();
  const sessionID = props.embeddedSessionID;
  const sessions = useTerminalStore((s) => s.sessions);
  const session = sessions.find((s) => s.id === sessionID);
  const hostID = session?.hostID ?? "";
  const pendingApp = useUIStore((s) => s.pendingApp);
  const setPendingApp = useUIStore((s) => s.setPendingApp);

  const { data: catalog } = useQuery({
    queryKey: ["apps", "catalog"],
    queryFn: appsApi.catalog,
  });
  const { data: installations } = useQuery({
    queryKey: ["apps", "installations", hostID],
    queryFn: () => appsApi.installations(hostID),
    enabled: hostID !== "",
  });

  // 面板内三态：列表 / 安装向导 / 管理某个已安装应用。
  const [wizard, setWizard] = useState<WizardState | null>(null);
  const [manageAppID, setManageAppID] = useState<string | null>(null);

  /** 打开安装向导；若该应用有进行中的任务则直接附加到执行进度。 */
  const openWizard = (entry: CatalogEntry) => {
    appsApi
      .tasks(hostID, 20)
      .then((tasks) => {
        const running = (tasks ?? []).find(
          (t) => t.appId === entry.spec.id && t.hostId === hostID && t.status === "running",
        );
        setManageAppID(null);
        if (running) {
          setWizard({ entry, phase: "running", inputs: {}, task: running });
        } else {
          setWizard({ entry, phase: "inputs", inputs: {}, task: null });
        }
      })
      .catch(() => setWizard({ entry, phase: "inputs", inputs: {}, task: null }));
  };

  // 消费一级页的跳转意图：本主机的应用 → 已安装则进入管理，否则进入向导。
  useEffect(() => {
    if (!pendingApp || pendingApp.hostId !== hostID) return;
    const appID = pendingApp.appId;
    setPendingApp(null);
    const installed = (installations ?? []).some((i) => i.appId === appID);
    if (installed) {
      setWizard(null);
      setManageAppID(appID);
    } else {
      setManageAppID(null);
      appsApi
        .app(appID)
        .then((entry) => openWizard(entry))
        .catch((e) => toast.error(errorMessage(e)));
    }
  }, [pendingApp, hostID, installations, setPendingApp]);

  if (!session) {
    // 会话还在建立（连接中尚未注册）：应用面板稍后随会话就绪渲染。
    return <div className="h-full w-full" />;
  }
  if (isLocalSession(session)) {
    return (
      <div className="flex h-full items-center justify-center p-6 text-center text-sm text-muted-foreground">
        {t("apps.localSession")}
      </div>
    );
  }

  if (wizard) {
    return (
      <InstallWizard
        sessionID={sessionID}
        hostID={hostID}
        state={wizard}
        onChange={setWizard}
        onDone={() => setWizard(null)}
      />
    );
  }

  if (manageAppID) {
    const entry = (catalog ?? []).find((e) => e.spec.id === manageAppID);
    if (entry) {
      return (
        <div className="flex h-full flex-col overflow-auto p-3">
          <Button size="sm" variant="ghost" className="mb-2 self-start"
            onClick={() => setManageAppID(null)}>
            <ArrowLeft className="size-4" /> {t("apps.backToApps")}
          </Button>
          <AppManagePanel
            sessionID={sessionID}
            hostID={hostID}
            spec={entry.spec}
            onUninstalled={() => setManageAppID(null)}
          />
        </div>
      );
    }
  }

  return (
    <div className="flex h-full flex-col gap-4 overflow-auto p-3">
      <section>
        <h3 className="mb-2 flex items-center gap-1.5 text-xs font-medium text-muted-foreground">
          <Package className="size-3.5" /> {t("apps.title")}
        </h3>
        <div className="flex flex-col gap-1.5">
          {(catalog ?? []).map((entry) => {
            const inst = (installations ?? []).find((i) => i.appId === entry.spec.id);
            return (
              <CatalogRow
                key={entry.spec.id}
                entry={entry}
                installed={inst?.status === "installed"}
                installing={inst?.status === "installing"}
                onInstall={() => openWizard(entry)}
                onManage={() => setManageAppID(entry.spec.id)}
              />
            );
          })}
        </div>
      </section>
      <p className="mt-auto text-[11px] text-muted-foreground">{t("apps.operationsHint")}</p>
    </div>
  );
}

/** 目录行：图标 + 名称 + 状态徽章 + 安装/管理按钮。 */
function CatalogRow(props: {
  entry: CatalogEntry;
  installed: boolean;
  installing: boolean;
  onInstall: () => void;
  onManage: () => void;
}) {
  const { t } = useTranslation();
  const spec = props.entry.spec;
  const { data: icon } = useQuery({
    queryKey: ["apps", "icon", spec.id, spec.icon],
    queryFn: () => appsApi.icon(spec.id, spec.icon || ""),
    enabled: !!spec.icon,
    staleTime: Infinity,
  });
  const iconUri = icon || null;
  return (
    <div className="flex items-center gap-2 rounded-md border border-border bg-card px-2.5 py-2">
      {iconUri ? (
        <img src={iconUri} alt="" className="size-7 shrink-0 rounded-md" />
      ) : (
        <Package className="size-6 shrink-0 text-muted-foreground" />
      )}
      <div className="min-w-0 flex-1">
        <div className="truncate text-sm font-medium">{spec.name}</div>
        <div className="truncate text-[11px] text-muted-foreground">{spec.description}</div>
      </div>
      {props.installing ? (
        <Button size="sm" variant="outline" onClick={props.onInstall} title={t("apps.viewProgress")}>
          <Loader2 className="size-3.5 animate-spin" /> {t("apps.installing")}
        </Button>
      ) : props.installed ? (
        <Button size="sm" variant="outline" onClick={props.onManage}>
          <Settings2 className="size-3.5" /> {t("apps.manageAction")}
          <ChevronRight className="size-3.5" />
        </Button>
      ) : (
        <Button size="sm" onClick={props.onInstall}>
          <Rocket className="size-3.5" /> {t("apps.installAction")}
        </Button>
      )}
    </div>
  );
}

interface WizardState {
  entry: CatalogEntry;
  phase: "inputs" | "running";
  inputs: Record<string, string>;
  task: DeployTask | null;
}

/**
 * 安装向导（简化）：填参数 → 直接执行 → 只看过程输出。不展示命令清单，
 * 各步骤输出合并为一个实时日志面板；失败时保留向导，可查看输出并一键重试。
 */
function InstallWizard(props: {
  sessionID: string;
  hostID: string;
  state: WizardState;
  onChange: React.Dispatch<React.SetStateAction<WizardState | null>>;
  onDone: () => void;
}) {
  const { t } = useTranslation();
  const { sessionID, hostID, state } = props;
  const spec = state.entry.spec;
  const queryClient = useQueryClient();
  const outputRef = useRef<HTMLPreElement | null>(null);

  const finish = (ok: boolean) => {
    queryClient.invalidateQueries({ queryKey: ["apps", "installations"] });
    if (ok) {
      toast.success(t("apps.installDone"));
      setTimeout(() => props.onChange(null), 1500);
    } else {
      // 失败：保留向导（输出面板已展示原因），可一键重试。
      toast.error(t("apps.installFailedMsg"));
    }
  };

  const start = useMutation({
    mutationFn: async () => {
      const task = await appsApi.install(sessionID, hostID, spec.id, state.inputs);
      props.onChange({ ...state, phase: "running", task });
    },
    onError: (e) => {
      const msg = errorMessage(e);
      if (msg.includes("已有任务")) {
        // 同主机同应用已有任务在跑：转附加到该任务的执行进度。
        appsApi
          .tasks(hostID, 20)
          .then((tasks) => {
            const running = (tasks ?? []).find(
              (t) => t.appId === spec.id && t.hostId === hostID && t.status === "running",
            );
            if (running) props.onChange({ ...state, phase: "running", task: running });
            else toast.error(msg);
          })
          .catch(() => toast.error(msg));
        return;
      }
      toast.error(msg);
    },
  });

  // 执行阶段：订阅任务事件（步骤级实时更新），并以任务轮询兜底。
  useEffect(() => {
    if (state.phase !== "running") return;
    const taskID = state.task?.id;
    if (!taskID) return;
    const applyEvent = (ev: DeployTaskEvent) => {
      if (ev.taskId !== taskID) return;
      props.onChange((prev) => {
        if (!prev || prev.phase !== "running" || !prev.task) return prev;
        const steps = mergeStep(prev.task.steps ?? [], ev.step);
        const updated: DeployTask = { ...prev.task, status: ev.status, steps, lastError: ev.lastError ?? prev.task.lastError };
        if (ev.status !== "running") {
          setTimeout(() => finish(ev.status === "succeeded"), 300);
        }
        return { ...prev, task: updated };
      });
    };
    const cancel = Events.On("apps:task", (e: unknown) => {
      const ev = (e as { data?: DeployTaskEvent })?.data ?? (e as DeployTaskEvent);
      applyEvent(ev);
    });
    const poll = setInterval(() => {
      appsApi
        .task(taskID)
        .then((task) => {
          props.onChange((prev) => (prev && prev.phase === "running" ? { ...prev, task } : prev));
          if (task.status !== "running") {
            clearInterval(poll);
            finish(task.status === "succeeded");
          }
        })
        .catch(() => {});
    }, 1500);
    return () => {
      (cancel as unknown as () => void)?.();
      clearInterval(poll);
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [state.phase, state.task?.id]);

  const steps = state.task?.steps ?? [];
  const taskFailed = state.task?.status === "failed";
  const taskSucceeded = state.task?.status === "succeeded";
  const runningIdx = steps.findIndex((s) => s.status === "running");
  // 当前执行到的步骤（步骤间隙回退到已开始的最后一步）。
  const progressIdx =
    runningIdx >= 0
      ? runningIdx
      : Math.min(steps.filter((s) => s.status !== "pending").length, steps.length - 1);
  // 已开始步骤的合并输出：命令（渲染后的最终文本）+ 输出；失败步骤展示错误。
  const combined = steps
    .filter((s) => s.status !== "pending")
    .map(
      (s) =>
        `── ${s.name} ──\n$ ${s.command ?? ""}\n${(s.status === "failed" ? s.error : s.output) || "…"}`,
    )
    .join("\n\n");

  // 输出追加时自动滚动到底部。
  useEffect(() => {
    const el = outputRef.current;
    if (el) el.scrollTop = el.scrollHeight;
  }, [combined]);

  return (
    <div className="flex h-full flex-col gap-2 overflow-auto p-3">
      <div className="flex items-center gap-2">
        {state.phase === "inputs" && (
          <Button size="sm" variant="ghost" onClick={() => props.onChange(null)}>
            <ArrowLeft className="size-4" />
          </Button>
        )}
        <span className="text-sm font-semibold">
          {t("apps.confirmInstallTitle", { name: spec.name })}
        </span>
      </div>

      {/* 参数输入：校验通过后直接开始执行 */}
      {state.phase === "inputs" && (
        <div className="flex flex-col gap-3">
          {spec.install.notice && (
            <div className="flex items-start gap-2 rounded-md border border-amber-500/40 bg-amber-500/10 p-2.5 text-xs text-amber-600 dark:text-amber-400">
              <AlertTriangle className="mt-0.5 size-4 shrink-0" />
              <span>{spec.install.notice}</span>
            </div>
          )}
          {(spec.install.inputs ?? []).map((input) => (
            <div key={input.key} className="flex flex-col gap-1">
              <Label htmlFor={`in-${input.key}`}>
                {input.label}
                {input.required && <span className="text-destructive"> *</span>}
              </Label>
              <Input
                id={`in-${input.key}`}
                type={input.type === "secret" ? "password" : "text"}
                placeholder={input.placeholder}
                value={state.inputs[input.key] ?? ""}
                onChange={(e) =>
                  props.onChange({ ...state, inputs: { ...state.inputs, [input.key]: e.target.value } })
                }
              />
            </div>
          ))}
          <Button
            className="mt-1"
            disabled={start.isPending}
            onClick={() => {
              for (const input of spec.install.inputs ?? []) {
                const v = (state.inputs[input.key] ?? "").trim();
                if (input.required && !v) {
                  toast.error(`${input.label}: ${input.placeholder || t("apps.inputsTitle")}`);
                  return;
                }
                if (v && input.validate?.startsWith("min=")) {
                  const min = Number(input.validate.slice(4));
                  if (min > 0 && [...v].length < min) {
                    toast.error(`${input.label} ≥ ${min}`);
                    return;
                  }
                }
              }
              start.mutate();
            }}
          >
            {start.isPending ? (
              <Loader2 className="size-4 animate-spin" />
            ) : (
              <Play className="size-4" />
            )}
            {t("apps.execute")}
          </Button>
        </div>
      )}

      {/* 执行中：进度行 + 实时输出面板 */}
      {state.phase === "running" && (
        <div className="flex min-h-0 flex-1 flex-col gap-2">
          <div className="flex items-center gap-2 text-xs">
            {taskSucceeded ? (
              <CheckCircle2 className="size-4 shrink-0 text-emerald-500" />
            ) : taskFailed ? (
              <XCircle className="size-4 shrink-0 text-destructive" />
            ) : (
              <Loader2 className="size-4 shrink-0 animate-spin text-primary" />
            )}
            <span
              className={cn(
                "truncate font-medium",
                taskFailed && "text-destructive",
                taskSucceeded && "text-emerald-500",
              )}
            >
              {taskSucceeded
                ? t("apps.installDone")
                : taskFailed
                  ? t("apps.installFailedMsg")
                  : t("apps.installProgress", {
                      i: progressIdx + 1,
                      n: steps.length,
                      name: steps[progressIdx]?.name ?? "",
                    })}
            </span>
          </div>
          <pre
            ref={outputRef}
            className="min-h-48 flex-1 overflow-auto rounded-md bg-zinc-900 p-3 text-[11px] leading-relaxed text-zinc-200"
          >
            {combined || "…"}
          </pre>
          {state.task?.lastError && (
            <p className="text-xs text-destructive">{state.task.lastError}</p>
          )}
          {taskFailed && (
            <div className="flex gap-2">
              <Button size="sm" onClick={() => start.mutate()} disabled={start.isPending}>
                <Play className="size-4" /> {t("apps.retry")}
              </Button>
              <Button size="sm" variant="outline" onClick={() => props.onChange(null)}>
                {t("common.close")}
              </Button>
            </div>
          )}
        </div>
      )}
    </div>
  );
}

function mergeStep(steps: DeployStepRun[], update: DeployStepRun | undefined | null): DeployStepRun[] {
  if (!update) return steps;
  const idx = steps.findIndex((s) => s.stepId === update.stepId);
  if (idx < 0) return steps;
  const next = [...steps];
  next[idx] = update;
  return next;
}
