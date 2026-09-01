// Tunnel feature API — typed wrappers over the generated TunnelService and
// domain model bindings.

import {
  TunnelService,
  type TunnelStatusDTO,
} from "@/../bindings/github.com/ai-remote/workspace/internal/interfaces";
import {
  TunnelState,
  TunnelType,
  type TunnelConfig,
} from "@/../bindings/github.com/ai-remote/workspace/internal/domain";

export {
  TunnelState,
  TunnelType,
  type TunnelConfig,
  type TunnelStatusDTO,
};

export const tunnelApi = {
  /** Full snapshot of every known tunnel (initial panel load). */
  list: () => TunnelService.ListTunnels().then((r) => r ?? []),
  start: (hostId: string) => TunnelService.StartTunnel(hostId),
  stop: (hostId: string) => TunnelService.StopTunnel(hostId),
  /** Busy LOCAL listen ports among the requested ones (save-time pre-check;
   *  ports held by the host's own tunnels are exempt on the backend). */
  checkPorts: (hostId: string, ports: number[]) =>
    TunnelService.CheckTunnelPorts(hostId, ports).then((r) => r ?? []),
};

export const TUNNEL_TYPES: TunnelType[] = [
  TunnelType.TunnelLocal,
  TunnelType.TunnelRemote,
  TunnelType.TunnelDynamic,
];

/** Zero config for form resets. New rules default to enabled. */
export const EMPTY_TUNNEL: TunnelConfig = {
  enabled: true,
  type: TunnelType.TunnelLocal,
  listenPort: 0,
  bindHost: "127.0.0.1",
  targetHost: "127.0.0.1",
  targetPort: 0,
};

/** Must mirror domain.TunnelConfig.Key on the backend — the identity that
 *  ties a configured rule to its live status. */
export function tunnelKey(c: TunnelConfig): string {
  return `${c.type}|${c.bindHost ?? ""}|${c.listenPort}|${c.targetHost ?? ""}|${c.targetPort ?? 0}`;
}

/** Store key for a status event (rule-scoped: several rules share a host). */
export function statusStoreKey(s: { hostId: string; key: string }): string {
  return `${s.hostId}|${s.key}`;
}

/**
 * Human-readable rule summary, Xshell-style:
 *   local:   `本机 127.0.0.1:3306 → 目标(db:3306，从服务器解析)`
 *   remote:  `服务器 0.0.0.0:8080 → 127.0.0.1:3000（从本机解析）`
 *   dynamic: `SOCKS5 127.0.0.1:1080`
 */
export function tunnelRuleText(c: TunnelConfig): string {
  const bind = `${c.bindHost || "127.0.0.1"}:${c.listenPort || "…"}`;
  if (c.type === "dynamic") return `SOCKS5 ${bind}`;
  const target = `${c.targetHost || "?"}:${c.targetPort || "?"}`;
  return `${bind} → ${target}`;
}

/**
 * The equivalent OpenSSH command line for a rule — pasting it into any
 * terminal reproduces the tunnel:
 *   ssh -N [-p port] -L|-R|-D [bind:]listen:target[:target] user@host
 * Non-default bind addresses are written out; 127.0.0.1 is left implicit
 * (that is ssh's own default). IPv6 literals get bracketed. Without the host
 * record (deleted mid-session) it degrades to the bare rule summary.
 */
export function tunnelSshCommand(
  c: TunnelConfig,
  host?: { host: string; port: number; username: string },
): string {
  if (!host) return tunnelRuleText(c);
  const addr = (h: string) => (h.includes(":") ? `[${h}]` : h);
  const server = `${addr(host.username)}@${addr(host.host)}`;
  const portFlag = host.port === 22 ? "" : ` -p ${host.port}`;
  const bind = c.bindHost && c.bindHost !== "127.0.0.1" ? `${addr(c.bindHost)}:` : "";
  let fwd: string;
  if (c.type === "dynamic") {
    fwd = `-D ${bind}${c.listenPort}`;
  } else {
    const target = `${addr(c.targetHost ?? "")}:${c.targetPort ?? 0}`;
    fwd = `${c.type === "remote" ? "-R" : "-L"} ${bind}${c.listenPort}:${target}`;
  }
  return `ssh -N${portFlag} ${fwd} ${server}`;
}
