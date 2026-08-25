// Monitor feature API — thin wrappers over the generated MonitorService
// bindings. Collection runs on the backend over the session's SSH connection.
import { MonitorService } from "@/../bindings/github.com/ai-remote/workspace/internal/interfaces";

export const monitorApi = {
  overview: (sessionID: string) => MonitorService.GetOverview(sessionID),
  processes: (sessionID: string) => MonitorService.GetProcesses(sessionID),
  ports: (sessionID: string) => MonitorService.GetPorts(sessionID),
};

/** Format a KB value as KB/MB/GB. */
export function fmtKB(kb: number): string {
  if (kb >= 1024 * 1024) return `${(kb / 1024 / 1024).toFixed(1)} GB`;
  if (kb >= 1024) return `${(kb / 1024).toFixed(1)} MB`;
  return `${Math.round(kb)} KB`;
}

/** Format a bytes-per-second rate as B/s → KB/s → MB/s → GB/s. */
export function fmtRate(bps: number): string {
  if (bps >= 1024 * 1024 * 1024) return `${(bps / 1024 / 1024 / 1024).toFixed(2)} GB/s`;
  if (bps >= 1024 * 1024) return `${(bps / 1024 / 1024).toFixed(1)} MB/s`;
  if (bps >= 1024) return `${(bps / 1024).toFixed(1)} KB/s`;
  return `${Math.round(bps)} B/s`;
}

/** Format seconds as "3天 4小时", "2小时 5分", or "42秒". */
export function fmtUptime(sec: number): string {
  const d = Math.floor(sec / 86400);
  const h = Math.floor((sec % 86400) / 3600);
  const m = Math.floor((sec % 3600) / 60);
  if (d > 0) return `${d}天 ${h}小时`;
  if (h > 0) return `${h}小时 ${m}分`;
  if (m > 0) return `${m}分`;
  return `${Math.round(sec)}秒`;
}
