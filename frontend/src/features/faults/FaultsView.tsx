import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { FileWarning, Loader2, Search, Trash2 } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select } from "@/components/ui/select";
import { AgentMarkdown } from "@/features/agent/AgentMarkdown";
import { faultsApi, FAULT_SEVERITIES, FAULT_STATUSES } from "@/features/faults/api";
import type { FaultReportDTO } from "@/features/faults/api";
import { useHosts } from "@/features/hosts/hooks";
import { useConfirm } from "@/lib/useConfirm";
import { toast, errorMessage } from "@/lib/toast";

const FAULTS_KEY = ["fault-reports"] as const;

/** Severity → badge tone (static classes; Tailwind JIT needs literals). */
const SEVERITY_BADGE: Record<string, string> = {
  info: "border-sky-500/40 bg-sky-500/10 text-sky-600 dark:text-sky-400",
  warning: "border-amber-500/40 bg-amber-500/10 text-amber-600 dark:text-amber-400",
  critical: "border-red-500/40 bg-red-500/10 text-red-600 dark:text-red-400",
};

const STATUS_BADGE: Record<string, string> = {
  open: "border-red-500/40 bg-red-500/10 text-red-600 dark:text-red-400",
  monitoring: "border-amber-500/40 bg-amber-500/10 text-amber-600 dark:text-amber-400",
  resolved: "border-emerald-500/40 bg-emerald-500/10 text-emerald-600 dark:text-emerald-400",
};

/**
 * FaultsView — the 故障报告 tracker (standalone nav page): host-attached
 * incident reports distilled from agent conversations. Filter by host,
 * severity, tracking status or keyword; open a report for the full
 * markdown; move it along the open → monitoring → resolved lifecycle.
 */
export function FaultsView() {
  const { t } = useTranslation();
  const { askConfirm } = useConfirm();
  const queryClient = useQueryClient();
  const { data: hosts } = useHosts();

  // Filters: keyword debounced ~300ms, the rest immediate.
  const [keywordInput, setKeywordInput] = useState("");
  const [keyword, setKeyword] = useState("");
  const [hostId, setHostId] = useState("");
  const [severity, setSeverity] = useState("");
  const [status, setStatus] = useState("");

  useMemo(() => {
    const timer = setTimeout(() => setKeyword(keywordInput.trim()), 300);
    return () => clearTimeout(timer);
  }, [keywordInput]);

  const { data: reports, isLoading } = useQuery({
    queryKey: [FAULTS_KEY, { hostId, severity, status, keyword }],
    queryFn: () => faultsApi.list({ hostId, severity, status, keyword }),
  });

  const [detail, setDetail] = useState<FaultReportDTO | null>(null);
  const openDetail = (report: FaultReportDTO) => {
    faultsApi
      .get(report.id)
      .then(setDetail)
      .catch((e) => toast.error(errorMessage(e)));
  };

  const refreshDetailAndList = (report: FaultReportDTO) => {
    setDetail(report);
    queryClient.invalidateQueries({ queryKey: FAULTS_KEY });
  };

  const handleStatus = (report: FaultReportDTO, next: string) => {
    faultsApi
      .setStatus(report.id, next)
      .then(refreshDetailAndList)
      .catch((e) => toast.error(errorMessage(e)));
  };

  const handleDelete = async (report: FaultReportDTO) => {
    const ok = await askConfirm({
      title: t("faults.deleteTitle"),
      message: t("faults.deleteConfirm", { title: report.title }),
      danger: true,
      confirmLabel: t("common.delete"),
    });
    if (!ok) return;
    try {
      await faultsApi.remove(report.id);
      setDetail(null);
      queryClient.invalidateQueries({ queryKey: FAULTS_KEY });
      toast.info(t("faults.deleted"));
    } catch (e) {
      toast.error(errorMessage(e));
    }
  };

  return (
    <div className="flex h-full flex-col overflow-auto">
      <div className="mx-auto flex w-full max-w-4xl flex-col gap-4 p-4">
        <div className="flex items-center gap-2">
          <FileWarning className="h-5 w-5 text-primary" />
          <h2 className="text-lg font-semibold">{t("faults.title")}</h2>
        </div>

        <Card>
          <CardHeader>
            <CardTitle className="text-sm">{t("faults.title")}</CardTitle>
            <CardDescription>{t("faults.desc")}</CardDescription>
          </CardHeader>
          <CardContent className="flex flex-col gap-3">
            {/* Filter bar: keyword / host / severity / status. */}
            <div className="grid grid-cols-1 gap-2 sm:grid-cols-2 lg:grid-cols-4">
              <div className="relative">
                <Search className="pointer-events-none absolute left-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground" />
                <Input
                  value={keywordInput}
                  onChange={(e) => setKeywordInput(e.target.value)}
                  placeholder={t("faults.keywordPlaceholder")}
                  aria-label={t("faults.keyword")}
                  className="pl-8"
                />
              </div>
              <Select
                value={hostId}
                onChange={(e) => setHostId(e.target.value)}
                aria-label={t("faults.host")}
              >
                <option value="">{t("faults.allHosts")}</option>
                {(hosts ?? []).map((h) => (
                  <option key={h.id} value={h.id}>{h.name}</option>
                ))}
              </Select>
              <Select
                value={severity}
                onChange={(e) => setSeverity(e.target.value)}
                aria-label={t("faults.severity")}
              >
                <option value="">{t("faults.allSeverities")}</option>
                {FAULT_SEVERITIES.map((s) => (
                  <option key={s} value={s}>{t(`faults.severity_${s}`)}</option>
                ))}
              </Select>
              <Select
                value={status}
                onChange={(e) => setStatus(e.target.value)}
                aria-label={t("faults.status")}
              >
                <option value="">{t("faults.allStatuses")}</option>
                {FAULT_STATUSES.map((s) => (
                  <option key={s} value={s}>{t(`faults.status_${s}`)}</option>
                ))}
              </Select>
            </div>

            {isLoading ? (
              <div className="flex items-center justify-center py-10 text-muted-foreground">
                <Loader2 className="h-4 w-4 animate-spin" />
              </div>
            ) : !reports || reports.length === 0 ? (
              <div className="flex flex-col items-center gap-2 py-10 text-center">
                <FileWarning className="h-8 w-8 text-muted-foreground" />
                <p className="text-sm text-muted-foreground">{t("faults.empty")}</p>
                <p className="text-xs text-muted-foreground">{t("faults.emptyHint")}</p>
              </div>
            ) : (
              <div className="flex flex-col gap-1.5">
                {reports.map((r) => (
                  <button
                    key={r.id}
                    type="button"
                    onClick={() => openDetail(r)}
                    className="flex items-center gap-2.5 rounded-[var(--radius)] border border-border px-3 py-2.5 text-left transition-colors hover:bg-accent/50"
                  >
                    <div className="min-w-0 flex-1">
                      <div className="flex items-center gap-2">
                        <span className="truncate text-sm font-medium">{r.title}</span>
                        <Badge
                          variant="outline"
                          className={`shrink-0 text-[10px] ${SEVERITY_BADGE[r.severity] ?? ""}`}
                        >
                          {t(`faults.severity_${r.severity}`, r.severity)}
                        </Badge>
                        <Badge
                          variant="outline"
                          className={`shrink-0 text-[10px] ${STATUS_BADGE[r.status] ?? ""}`}
                        >
                          {t(`faults.status_${r.status}`, r.status)}
                        </Badge>
                      </div>
                      <p className="mt-0.5 truncate text-xs text-muted-foreground">
                        {r.hostName || t("faults.local")} · {t("faults.updatedAt", { time: new Date(r.updatedAt).toLocaleString() })}
                      </p>
                    </div>
                  </button>
                ))}
              </div>
            )}
          </CardContent>
        </Card>
      </div>

      {/* Report detail: full markdown + lifecycle controls. */}
      <Dialog open={!!detail} onOpenChange={(o) => !o && setDetail(null)}>
        <DialogContent className="flex max-h-[85vh] max-w-2xl flex-col overflow-auto">
          <DialogHeader>
            <DialogTitle className="flex flex-wrap items-center gap-2 pr-6">
              <span>{detail?.title}</span>
              {detail && (
                <>
                  <Badge variant="outline" className={`text-[10px] ${SEVERITY_BADGE[detail.severity] ?? ""}`}>
                    {t(`faults.severity_${detail.severity}`, detail.severity)}
                  </Badge>
                  <Badge variant="outline" className={`text-[10px] ${STATUS_BADGE[detail.status] ?? ""}`}>
                    {t(`faults.status_${detail.status}`, detail.status)}
                  </Badge>
                </>
              )}
            </DialogTitle>
            <DialogDescription>
              {detail ? `${detail.hostName || t("faults.local")} · ${t("faults.updatedAt", { time: new Date(detail.updatedAt).toLocaleString() })}` : ""}
            </DialogDescription>
          </DialogHeader>
          <div className="min-h-0 flex-1 overflow-auto rounded-[var(--radius)] border border-border bg-secondary/30 px-4 py-3 text-sm">
            {detail && <AgentMarkdown content={detail.body} canInsert={false} onInsert={() => {}} />}
          </div>
          <DialogFooter className="items-center gap-2">
            {detail && (
              <>
                <Button
                  variant="ghost"
                  size="sm"
                  className="mr-auto text-destructive hover:text-destructive"
                  onClick={() => void handleDelete(detail)}
                >
                  <Trash2 className="h-3.5 w-3.5" />
                  {t("common.delete")}
                </Button>
                <Label className="text-xs text-muted-foreground">{t("faults.status")}</Label>
                <Select
                  value={detail.status}
                  onChange={(e) => handleStatus(detail, e.target.value)}
                  className="h-8 w-32 text-xs"
                >
                  {FAULT_STATUSES.map((s) => (
                    <option key={s} value={s}>{t(`faults.status_${s}`)}</option>
                  ))}
                </Select>
              </>
            )}
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
