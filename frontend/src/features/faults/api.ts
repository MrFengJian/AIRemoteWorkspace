// Faults feature API — typed wrappers over the generated FaultReportService
// bindings (host-attached incident reports).

import {
  FaultReportService,
  type FaultReportDTO,
  type FaultReportFilterDTO,
} from "@/../bindings/github.com/ai-remote/workspace/internal/interfaces";

export type { FaultReportDTO, FaultReportFilterDTO };

/** Tracking statuses (domain-normalized server-side). */
export const FAULT_STATUSES = ["open", "monitoring", "resolved"] as const;
/** Severity levels (domain-normalized server-side). */
export const FAULT_SEVERITIES = ["info", "warning", "critical"] as const;

export const faultsApi = {
  /** Reports matching the filter, newest update first. */
  list: (filter: Partial<FaultReportFilterDTO>) =>
    FaultReportService.ListReports({
      hostId: filter.hostId ?? "",
      severity: filter.severity ?? "",
      status: filter.status ?? "",
      keyword: filter.keyword ?? "",
    }).then((r) => r ?? []),
  /** One report with its full markdown body. */
  get: (id: string) => FaultReportService.GetReport(id),
  /** Create or update a report (severity/status normalize server-side). */
  save: (report: FaultReportDTO) => FaultReportService.SaveReport(report),
  /** Move a report along its tracking lifecycle. */
  setStatus: (id: string, status: string) =>
    FaultReportService.SetReportStatus(id, status),
  remove: (id: string) => FaultReportService.DeleteReport(id),
};
