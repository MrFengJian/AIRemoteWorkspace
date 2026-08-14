// Settings feature API — typed wrappers over the generated bindings, plus
// pure presentation helpers.
//
// Domain types (AppConfig, SecurityMode, ModelProviderDTO, …) come from the
// generated Wails bindings — do not redeclare them here.

import {
  ModelProviderService,
  type ModelProviderDTO,
  type SaveProviderInput,
  type TestConnectionResult,
  type TestProviderInput,
} from "@/../bindings/github.com/ai-remote/workspace/internal/interfaces";

export type { ModelProviderDTO, SaveProviderInput, TestProviderInput, TestConnectionResult };

/** Model-provider management (Settings → Models). */
export const providersApi = {
  list: () => ModelProviderService.ListProviders(),
  save: (input: SaveProviderInput) => ModelProviderService.SaveProvider(input),
  remove: (id: string) => ModelProviderService.DeleteProvider(id),
  test: (input: TestProviderInput) => ModelProviderService.TestProvider(input),
  fetchModels: (input: TestProviderInput) => ModelProviderService.FetchModels(input),
};

/**
 * Human-readable labels for each security mode value.
 * Keys are the string values of the generated SecurityMode enum, so they
 * stay decoupled from the enum's member names.
 */
export const SECURITY_MODE_LABELS: Record<string, string> = {
  convenience: "Convenience",
  balanced: "Balanced (default)",
  secure: "Secure",
};
