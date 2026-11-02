import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { Browser } from "@wailsio/runtime";
import {
  CheckCircle2,
  CircleStop,
  ExternalLink,
  Eye,
  EyeOff,
  KeyRound,
  Play,
  RefreshCw,
  RotateCw,
  ShieldCheck,
  Trash2,
  XCircle,
  ArrowUpCircle,
} from "lucide-react";

import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useConfirm } from "@/lib/useConfirm";
import { toast, errorMessage } from "@/lib/toast";
import {
  appsApi,
  formatMemKB,
  formatUptime,
  stateKey,
  type AppCredential,
  type AppSpec,
  type HostAppStatus,
} from "./api";

/**
 * 应用管理面板（会话内）：还原运维面板布局——服务状态三卡（状态/端口/PID）
 * + 运行时长/内存行 + 操作按钮组（打开面板/登录凭证/启动/重启/停止/开放端口
 * /升级/卸载）+ 服务日志区（最近 200 行 + 刷新）。所有指标/按钮一一映射到
 * app.json 操作手册的命令，经当前会话 SSH 通道在远端执行。
 */
export function AppManagePanel(props: {
  sessionID: string;
  hostID: string;
  spec: AppSpec;
  onUninstalled?: () => void;
}) {
  const { t } = useTranslation();
  const { sessionID, hostID, spec } = props;
  const { askConfirm } = useConfirm();
  const queryClient = useQueryClient();
  const [credsOpen, setCredsOpen] = useState(false);

  const status = useQuery({
    queryKey: ["apps", "status", sessionID, spec.id],
    queryFn: () => appsApi.status(sessionID, spec.id),
    refetchInterval: 5_000,
    retry: false,
  });
  const logs = useQuery({
    queryKey: ["apps", "logs", sessionID, spec.id],
    queryFn: () => appsApi.logs(sessionID, spec.id, 200),
  });

  const invalidate = () => {
    queryClient.invalidateQueries({ queryKey: ["apps", "status", sessionID, spec.id] });
    queryClient.invalidateQueries({ queryKey: ["apps", "logs", sessionID, spec.id] });
    queryClient.invalidateQueries({ queryKey: ["apps", "installations"] });
  };

  const control = useMutation({
    mutationFn: (action: "start" | "stop" | "restart") =>
      appsApi.control(sessionID, spec.id, action),
    onSuccess: () => setTimeout(invalidate, 1500),
    onError: (e) => toast.error(errorMessage(e)),
  });
  const openPort = useMutation({
    mutationFn: () => appsApi.openPort(sessionID, spec.id),
    onSuccess: (out) => {
      toast.success(`${t("apps.portOpened")}${out?.trim() ? `: ${out.trim()}` : ""}`);
    },
    onError: (e) => toast.error(errorMessage(e)),
  });
  const upgrade = useMutation({
    mutationFn: () => appsApi.update(sessionID, hostID, spec.id),
    onSuccess: () => toast.success(t("apps.taskRunning")),
    onError: (e) => toast.error(errorMessage(e)),
  });
  const uninstall = useMutation({
    mutationFn: async () => {
      const ok = await askConfirm({        title: t("apps.uninstall"),
        message: `${spec.name}: ${t("apps.confirmInstallHint")}`,
        danger: true,
        confirmLabel: t("apps.uninstall"),
      });
      if (!ok) return null;
      return appsApi.uninstall(sessionID, hostID, spec.id);
    },
    onSuccess: (task) => {
      if (task) toast.success(t("apps.taskRunning"));
      props.onUninstalled?.();
      invalidate();
    },
    onError: (e) => toast.error(errorMessage(e)),
  });

  const st: HostAppStatus | undefined = status.data;
  const stateColor =
    st?.state === "running"
      ? "text-emerald-500"
      : st?.state === "failed"
        ? "text-red-500"
        : "text-muted-foreground";

  return (
    <div className="flex flex-col gap-4">
      {/* 服务状态三卡 */}
      <div className="rounded-lg border border-border bg-card p-4">
        <div className="mb-3 flex items-center justify-between">
          <h3 className="flex items-center gap-2 text-sm font-semibold">
            <span className="h-3.5 w-1 rounded bg-primary" />
            {spec.name} {t("apps.statusTitle")}
          </h3>
        </div>
        <div className="grid grid-cols-3 gap-3">
          <StatusCard
            icon={st?.state === "running" ? CheckCircle2 : XCircle}
            iconClass={stateColor}
            value={t(stateKey(st?.state))}
            label={t("apps.statusTitle")}
          />
          <StatusCard
            icon={ExternalLink}
            iconClass="text-blue-500"
            value={st?.port ? `:${st.port}` : spec.panel?.port ? `:${spec.panel.port}` : "-"}
            label={t("apps.listenPort")}
          />
          <StatusCard
            icon={ShieldCheck}
            iconClass="text-amber-500"
            value={st?.pid ? String(st.pid) : "-"}
            label={t("apps.processPid")}
          />
        </div>
        <div className="mt-3 flex gap-4 text-xs text-muted-foreground">
          <span>
            {t("apps.uptime")}: <span className="text-foreground">{formatUptime(st?.uptimeSec ?? 0)}</span>
          </span>
          <span>
            {t("apps.memory")}: <span className="text-foreground">{formatMemKB(st?.memKb ?? 0)}</span>
          </span>
          {st?.version && (
            <span>
              VER: <span className="text-foreground">{st.version}</span>
            </span>
          )}
        </div>

        {/* 操作按钮组 */}
        <div className="mt-4 flex flex-wrap gap-2">
          <OpenPanelButton hostID={hostID} spec={spec} />
          <Button size="sm" variant="outline" onClick={() => setCredsOpen(true)}
            disabled={!spec.panel || (spec.panel.credentials ?? []).length === 0}>
            <KeyRound className="size-4" /> {t("apps.credentials")}
          </Button>
          <Button size="sm" className="bg-emerald-600 text-white hover:bg-emerald-600/90"
            disabled={control.isPending} onClick={() => control.mutate("start")}>
            <Play className="size-4" /> {t("apps.start")}
          </Button>
          <Button size="sm" className="bg-amber-500 text-white hover:bg-amber-500/90"
            disabled={control.isPending} onClick={() => control.mutate("restart")}>
            <RotateCw className="size-4" /> {t("apps.restart")}
          </Button>
          <Button size="sm" variant="destructive" disabled={control.isPending}
            onClick={() => control.mutate("stop")}>
            <CircleStop className="size-4" /> {t("apps.stop")}
          </Button>
          <Button size="sm" variant="outline" disabled={!spec.ops.openPort || openPort.isPending}
            onClick={() => openPort.mutate()}>
            <ShieldCheck className="size-4" /> {t("apps.openPort")}
          </Button>
          {spec.ops.update && (
            <Button size="sm" variant="outline" disabled={upgrade.isPending}
              onClick={() => upgrade.mutate()}>
              <ArrowUpCircle className="size-4" /> {t("apps.upgrade")}
            </Button>
          )}
          <Button size="sm" variant="outline" disabled={uninstall.isPending}
            onClick={() => uninstall.mutate()}>
            <Trash2 className="size-4" /> {t("apps.uninstall")}
          </Button>
        </div>
      </div>

      {/* 服务日志 */}
      <div className="rounded-lg border border-border bg-card p-4">
        <div className="mb-2 flex items-center justify-between">
          <h3 className="flex items-center gap-2 text-sm font-semibold">
            <span className="h-3.5 w-1 rounded bg-primary" />
            {t("apps.serviceLogs")}
            <span className="text-xs font-normal text-muted-foreground">
              {t("apps.recentLines", { n: 200 })}
            </span>
          </h3>
          <Button size="sm" variant="outline" onClick={() => logs.refetch()} disabled={logs.isFetching}>
            <RefreshCw className={logs.isFetching ? "size-4 animate-spin" : "size-4"} /> {t("apps.refresh")}
          </Button>
        </div>
        <pre className="max-h-64 overflow-auto rounded-md bg-zinc-900 p-3 text-[11px] leading-relaxed text-zinc-200">
          {logs.data?.trim() || "…"}
        </pre>
      </div>

      <CredentialsDialog open={credsOpen} onOpenChange={setCredsOpen}
        hostID={hostID} appID={spec.id} fields={spec.panel?.credentials ?? []} />
    </div>
  );
}

function StatusCard(props: {
  icon: typeof CheckCircle2;
  iconClass: string;
  value: string;
  label: string;
}) {
  const Icon = props.icon;
  return (
    <div className="flex items-center gap-2 rounded-md border border-border/60 bg-background/50 p-3">
      <Icon className={`size-5 shrink-0 ${props.iconClass}`} />
      <div className="min-w-0">
        <div className="truncate text-base font-semibold">{props.value}</div>
        <div className="truncate text-[11px] text-muted-foreground">{props.label}</div>
      </div>
    </div>
  );
}

/** 打开面板：解析面板地址后新窗口打开。 */
function OpenPanelButton(props: { hostID: string; spec: AppSpec }) {
  const { t } = useTranslation();
  const spec = props.spec;
  if (!spec.panel || !spec.panel.url) {
    return (
      <Button size="sm" variant="outline" disabled title={t("apps.noPanel")}>
        <ExternalLink className="size-4" /> {t("apps.openPanel")}
      </Button>
    );
  }
  const open = async () => {
    try {
      const url = await appsApi.panelURL(props.hostID, spec.id);
      await Browser.OpenURL(url);
    } catch (e) {
      toast.error(errorMessage(e));
    }
  };
  return (
    <Button size="sm" onClick={open}>
      <ExternalLink className="size-4" /> {t("apps.openPanel")}
    </Button>
  );
}

/** 登录凭证对话框：按 spec.panel.credentials 字段读写 OS 钥匙串。 */
function CredentialsDialog(props: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  hostID: string;
  appID: string;
  fields: AppCredential[];
}) {
  const { t } = useTranslation();
  const [values, setValues] = useState<Record<string, string>>({});
  // secret 字段的明文查看开关（凭证是用户自己设置的，必须能看到原文）。
  const [revealed, setRevealed] = useState<Record<string, boolean>>({});
  const queryClient = useQueryClient();

  useQuery({
    queryKey: ["apps", "credentials", props.hostID, props.appID],
    queryFn: async () => {
      const saved = await appsApi.credentials(props.hostID, props.appID);
      // 已保存值优先；未保存的字段回退到 spec 声明的默认值（如用户名 admin）。
      const merged: Record<string, string> = {};
      for (const f of props.fields) merged[f.key] = f.default ?? "";
      for (const [k, v] of Object.entries(saved ?? {})) {
        if (v) merged[k] = v;
      }
      setValues(merged);
      return saved;
    },
    enabled: props.open,
  });

  const save = useMutation({
    mutationFn: () => appsApi.saveCredentials(props.hostID, props.appID, values),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["apps", "credentials", props.hostID, props.appID] });
      toast.success(t("apps.save"));
      props.onOpenChange(false);
    },
    onError: (e) => toast.error(errorMessage(e)),
  });

  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      <DialogContent className="sm:max-w-sm">
        <DialogHeader>
          <DialogTitle>{t("apps.credentialsTitle")}</DialogTitle>
        </DialogHeader>
        <p className="text-xs text-muted-foreground">{t("apps.credentialsHint")}</p>
        <div className="flex flex-col gap-3">
          {props.fields.map((f) => (
            <div key={f.key} className="grid grid-cols-[6rem_1fr] items-center gap-2">
              <Label htmlFor={`cred-${f.key}`}>{f.label}</Label>
              <div className="flex items-center gap-1">
                <Input
                  id={`cred-${f.key}`}
                  type={f.secret && !revealed[f.key] ? "password" : "text"}
                  value={values[f.key] ?? ""}
                  onChange={(e) => setValues((v) => ({ ...v, [f.key]: e.target.value }))}
                />
                {f.secret && (
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    className="shrink-0 px-2"
                    title={revealed[f.key] ? t("apps.hideCredential") : t("apps.showCredential")}
                    onClick={() => setRevealed((r) => ({ ...r, [f.key]: !r[f.key] }))}
                  >
                    {revealed[f.key] ? <EyeOff className="size-4" /> : <Eye className="size-4" />}
                  </Button>
                )}
              </div>
            </div>
          ))}
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => props.onOpenChange(false)}>
            {t("apps.cancel")}
          </Button>
          <Button onClick={() => save.mutate()} disabled={save.isPending}>
            {t("apps.save")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
