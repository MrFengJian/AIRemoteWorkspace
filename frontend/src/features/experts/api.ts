// Experts feature API — typed wrappers over the generated ExpertService
// bindings (the digital-employee roster).

import {
  ExpertService,
  type ExpertDTO,
} from "@/../bindings/github.com/ai-remote/workspace/internal/interfaces";

export type { ExpertDTO };

export const expertsApi = {
  /** Visible roster (dismissed builtins hidden), sorted by SortOrder. */
  list: () => ExpertService.ListExperts().then((r) => r ?? []),
  /** One expert including its full persona prompt (editor use). */
  get: (id: string) => ExpertService.GetExpert(id),
  /** Create or update an expert; returns the stored row. */
  save: (expert: ExpertDTO) => ExpertService.SaveExpert(expert),
  /** Delete an expert (builtins are dismissed, not destroyed). */
  remove: (id: string) => ExpertService.DeleteExpert(id),
};
