// Experts feature API — typed wrappers over the generated ExpertService
// bindings (the digital-employee roster).

import {
  ExpertService,
  type ExpertDTO,
  type ExpertExportResultDTO,
} from "@/../bindings/github.com/ai-remote/workspace/internal/interfaces";

export type { ExpertDTO, ExpertExportResultDTO };

export const expertsApi = {
  /** Visible roster (dismissed builtins hidden), sorted by SortOrder. */
  list: () => ExpertService.ListExperts().then((r) => r ?? []),
  /** One expert including its full persona prompt (editor use). */
  get: (id: string) => ExpertService.GetExpert(id),
  /** Create or update an expert; returns the stored row. */
  save: (expert: ExpertDTO) => ExpertService.SaveExpert(expert),
  /** Delete an expert (builtins are dismissed, not destroyed). */
  remove: (id: string) => ExpertService.DeleteExpert(id),
  /** Package the expert + its bound skill packs into a zip archive. */
  exportPackage: (id: string, zipPath: string) =>
    ExpertService.ExportExpert(id, zipPath),
  /** Restore an expert package (new custom row + skill packs installed). */
  importPackage: (zipPath: string) => ExpertService.ImportExpert(zipPath),
};
