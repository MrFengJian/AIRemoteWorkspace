// Apps feature API — thin wrappers over the generated AppDeployService
// bindings, plus small view helpers (icon data-URI, uptime/memory formatting
// mirrors of the backend's).
import { AppDeployService } from "@/../bindings/github.com/ai-remote/workspace/internal/interfaces";
import type {
  AppCredential,
  AppInput,
  AppSpec,
  CatalogEntry,
  CatalogMeta,
  CatalogUpdateCheck,
  DeployStep,
  DeployStepRun,
  DeployTask,
  DeployTaskEvent,
  HostAppStatus,
  Installation,
} from "@/../bindings/github.com/ai-remote/workspace/internal/domain/models";

export type {
  AppCredential,
  AppInput,
  AppSpec,
  CatalogEntry,
  CatalogMeta,
  CatalogUpdateCheck,
  DeployStep,
  DeployStepRun,
  DeployTask,
  DeployTaskEvent,
  HostAppStatus,
  Installation,
};

export type CatalogSource = "release" | "main";

export const appsApi = {
  catalog: () => AppDeployService.ListCatalog(),
  app: (id: string) => AppDeployService.GetApp(id),
  guide: (id: string) => AppDeployService.GetGuide(id),
  icon: (id: string, file: string) => AppDeployService.GetIcon(id, file), // 返回 data URL（官方原图 base64）
  catalogVersion: () => AppDeployService.GetCatalogVersion(),
  checkUpdate: (source: CatalogSource) => AppDeployService.CheckCatalogUpdate(source),
  applyUpdate: (source: CatalogSource) => AppDeployService.ApplyCatalogUpdate(source),
  detect: (sessionID: string, appID: string) => AppDeployService.DetectInstalled(sessionID, appID),
  install: (sessionID: string, hostID: string, appID: string, inputs: Record<string, string>) =>
    AppDeployService.StartInstall(sessionID, hostID, appID, inputs),
  uninstall: (sessionID: string, hostID: string, appID: string) =>
    AppDeployService.StartUninstall(sessionID, hostID, appID),
  update: (sessionID: string, hostID: string, appID: string) =>
    AppDeployService.StartUpdate(sessionID, hostID, appID),
  task: (id: string) => AppDeployService.GetTask(id),
  tasks: (hostID: string, limit: number) => AppDeployService.ListTasks(hostID, limit),
  status: (sessionID: string, appID: string) => AppDeployService.GetStatus(sessionID, appID),
  statusByHost: (hostID: string, appID: string) => AppDeployService.GetStatusByHost(hostID, appID),
  logs: (sessionID: string, appID: string, lines: number) =>
    AppDeployService.GetLogs(sessionID, appID, lines),
  control: (sessionID: string, appID: string, action: "start" | "stop" | "restart") =>
    AppDeployService.Control(sessionID, appID, action),
  openPort: (sessionID: string, appID: string) => AppDeployService.OpenPort(sessionID, appID),
  panelURL: (hostID: string, appID: string) => AppDeployService.GetPanelURL(hostID, appID),
  saveCredentials: (hostID: string, appID: string, values: Record<string, string>) =>
    AppDeployService.SavePanelCredential(hostID, appID, values),
  credentials: (hostID: string, appID: string) => AppDeployService.GetPanelCredential(hostID, appID),
  installations: (hostID: string) => AppDeployService.ListInstallations(hostID),
};

/** 秒 → "1-16:54:39"（与后端 FormatUptime 同规则；事件/轮询间隙本地兜底）。 */
export function formatUptime(sec: number): string {
  if (!sec || sec <= 0) return "-";
  const d = Math.floor(sec / 86400);
  const h = Math.floor((sec % 86400) / 3600);
  const m = Math.floor((sec % 3600) / 60);
  const s = Math.floor(sec % 60);
  const hms = `${h}:${String(m).padStart(2, "0")}:${String(s).padStart(2, "0")}`;
  return d > 0 ? `${d}-${hms}` : hms;
}

/** KB → 人类可读内存（740.1 MB 风格）。 */
export function formatMemKB(kb: number): string {
  if (!kb || kb <= 0) return "-";
  if (kb >= 1024 * 1024) return `${(kb / 1024 / 1024).toFixed(1)} GB`;
  if (kb >= 1024) return `${(kb / 1024).toFixed(1)} MB`;
  return `${kb} KB`;
}

/** HostAppStatus.state 的展示键。 */
export function stateKey(state: string | undefined): string {
  switch (state) {
    case "running":
      return "apps.stateRunning";
    case "stopped":
      return "apps.stateStopped";
    case "failed":
      return "apps.stateFailed";
    default:
      return "apps.stateUnknown";
  }
}
