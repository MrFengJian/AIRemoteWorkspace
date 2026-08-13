// Hosts feature API — typed wrappers over the generated HostService bindings.
//
// The bindings live under the package path matching go.mod; importing the
// namespace re-exports the generated call functions (CreateHost, ListHosts…).

import {
  HostService,
  type HostDTO,
  type HostInputDTO,
  type CredentialsDTO,
  type TestConnectionResult,
  type RememberedCredentialsDTO,
} from "@/../bindings/github.com/ai-remote/workspace/internal/interfaces";

export type {
  HostDTO,
  HostInputDTO,
  CredentialsDTO,
  TestConnectionResult,
  RememberedCredentialsDTO,
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
};

/** Auth type values match domain.AuthType on the backend. */
export const AUTH_TYPES = ["password", "key", "agent"] as const;
export type AuthType = (typeof AUTH_TYPES)[number];

export const AUTH_TYPE_LABELS: Record<string, string> = {
  password: "Password",
  key: "Private key file",
  agent: "ssh-agent",
};

/** Default host groups (custom groups are allowed by typing any value). */
export const HOST_GROUPS = ["", "test", "stage", "production"] as const;

export const GROUP_LABELS: Record<string, string> = {
  "": "None",
  test: "Test",
  stage: "Stage",
  production: "Production",
};
