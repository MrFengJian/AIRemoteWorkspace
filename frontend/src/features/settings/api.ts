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
