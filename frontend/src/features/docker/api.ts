// Docker feature API — thin wrappers over the generated DockerService
// bindings. Reads and actions run on the backend: over the session's SSH
// exec channel for remote tabs, or the local docker CLI for local terminals.
import { DockerService } from "@/../bindings/github.com/ai-remote/workspace/internal/interfaces";

/** Container lifecycle actions the panel may trigger (backend allowlist). */
export type DockerAction = "start" | "stop" | "restart" | "pause" | "unpause" | "kill";

export const dockerApi = {
  info: (sessionID: string) => DockerService.GetInfo(sessionID),
  containers: (sessionID: string, all: boolean) =>
    DockerService.ListContainers(sessionID, all),
  stats: (sessionID: string) => DockerService.GetContainerStats(sessionID),
  images: (sessionID: string) => DockerService.ListImages(sessionID),
  logs: (sessionID: string, container: string, tail: number) =>
    DockerService.GetLogs(sessionID, container, tail),
  action: (sessionID: string, container: string, action: DockerAction) =>
    DockerService.ContainerAction(sessionID, container, action),
};

export type DockerErrorKind = "notInstalled" | "daemonDown" | "generic";

/**
 * Map a backend error to a hint category. The sentinel messages come from
 * internal/application/docker_service.go (ErrDockerUnavailable /
 * ErrDockerDaemonDown).
 */
export function dockerErrorKind(e: unknown): DockerErrorKind {
  const msg = String((e as Error)?.message ?? e ?? "").toLowerCase();
  if (msg.includes("docker cli not available")) return "notInstalled";
  if (msg.includes("docker daemon unreachable")) return "daemonDown";
  return "generic";
}
