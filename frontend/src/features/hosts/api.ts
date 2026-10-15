// Hosts feature API — typed wrappers over the generated HostService bindings.
//
// The bindings live under the package path matching go.mod; importing the
// namespace re-exports the generated call functions (CreateHost, ListHosts…).

import {
  HostService,
  WindowService,
  type HostDTO,
  type HostInputDTO,
  type CredentialsDTO,
  type TestConnectionResult,
  type RememberedCredentialsDTO,
  type ImportedHostDTO,
  type ImportHostsResultDTO,
} from "@/../bindings/github.com/ai-remote/workspace/internal/interfaces";

export type {
  HostDTO,
  HostInputDTO,
  CredentialsDTO,
  TestConnectionResult,
  RememberedCredentialsDTO,
  ImportedHostDTO,
  ImportHostsResultDTO,
};

export const hostsApi = {
  list: () => HostService.ListHosts(),
  get: (id: string) => HostService.GetHost(id),
  create: (input: HostInputDTO) => HostService.CreateHost(input),
  update: (id: string, input: HostInputDTO) => HostService.UpdateHost(id, input),
  remove: (id: string) => HostService.DeleteHost(id),
  testConnection: (hostId: string, creds: CredentialsDTO) =>
    HostService.TestConnection(hostId, creds),
  saveCredentials: (hostId: string, creds: CredentialsDTO, remember: boolean) =>
    HostService.SaveCredentials(hostId, creds, remember),
  getRemembered: (hostId: string) =>
    HostService.GetRememberedCredentials(hostId),
  /** Open the standalone dual-pane SFTP window for the host. */
  openSftpWindow: (hostId: string) => WindowService.OpenSftpWindow(hostId),
  /** Import from other SSH clients: preview parsed sessions, then import the
   *  selected indexes (settings only — passwords/keys are never imported). */
  parseImport: (source: string, path: string) =>
    HostService.ParseHostImport(source, path),
  importHosts: (source: string, path: string, indexes: number[]) =>
    HostService.ImportHosts(source, path, indexes),
};

/** Import sources match the application-layer constants on the backend. */
export const IMPORT_SOURCES = ["xshell", "mobaxterm", "putty", "sshconfig"] as const;
export type ImportSource = (typeof IMPORT_SOURCES)[number];

/** Auth type values match domain.AuthType on the backend. Labels are
 *  localised via hosts.authType.* keys. */
export const AUTH_TYPES = ["password", "key", "agent"] as const;
export type AuthType = (typeof AUTH_TYPES)[number];

/** Default host groups (custom groups are allowed by typing any value).
 *  Displayed verbatim — only the "none" option is localised. */
export const HOST_GROUPS = ["", "test", "stage", "production"] as const;
