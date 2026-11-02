import { useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { Browser } from "@wailsio/runtime";
import {
  Globe,
  Loader2,
  Package,
  RefreshCw,
  BookOpen,
  Rocket,
  Settings2,
  ArrowUpCircle,
} from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Select } from "@/components/ui/select";
import { useUIStore } from "@/stores/ui.store";
import { useHosts } from "@/features/hosts/hooks";
import { useActivateOrConnect } from "@/features/hosts/useActivateOrConnect";
import { toast, errorMessage } from "@/lib/toast";
import {
  appsApi,
  type CatalogEntry,
  type CatalogMeta,
  type CatalogSource,
  type CatalogUpdateCheck,
  type Installation,
} from "./api";

/**
 * 应用部署一级页：目录浏览（图标/名称/简介/官网/标签）、目录更新检查
 * （release / main 分支）、全部主机已安装总览。安装与管理动作统一收敛到
 * 会话内：选主机 → 自动连接 → 跳转会话右侧应用面板。
 */
export function AppsView() {
  const { t } = useTranslation();
  const { data: catalog } = useQuery({ queryKey: ["apps", "catalog"], queryFn: appsApi.catalog });
  const { data: installations } = useQuery({
    queryKey: ["apps", "installations"],
    queryFn: () => appsApi.installations(""),
    refetchInterval: 30_000,
  });

  const installOn = useInstallOnHost();

  return (
    <div className="flex h-full w-full flex-col overflow-auto bg-background">
      <div className="mx-auto w-full max-w-5xl px-6 py-6">
        {/* Header + catalog toolbar */}
        <div className="flex flex-wrap items-end justify-between gap-3">
          <div>
            <h1 className="flex items-center gap-2 text-xl font-semibold">
              <Package className="size-5" /> {t("apps.title")}
            </h1>
            <p className="mt-1 text-sm text-muted-foreground">{t("apps.subtitle")}</p>
          </div>
          <CatalogToolbar />
        </div>

        {/* Catalog cards */}
        <div className="mt-6 grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-3">
          {(catalog ?? []).map((entry) => (
            <AppCard
              key={entry.spec.id}
              entry={entry}
              installed={(installations ?? []).some(
                (i) => i.appId === entry.spec.id && i.status === "installed",
              )}
              installing={(installations ?? []).some(
                (i) => i.appId === entry.spec.id && i.status === "installing",
              )}
              onInstall={() => installOn.open(entry.spec.id)}
              onManage={() => installOn.open(entry.spec.id)}
            />
          ))}
        </div>

        {/* Installed overview */}
        <InstalledOverview onOpen={installOn.open} />
      </div>
      {installOn.dialog}
    </div>
  );
}

/** 目录工具条：更新来源切换 + 检查/应用更新 + 当前版本展示。 */
function CatalogToolbar() {
  const { t } = useTranslation();
  const [source, setSource] = useState<CatalogSource>("release");
  const [checking, setChecking] = useState(false);
  const [applying, setApplying] = useState(false);
  const [available, setAvailable] = useState<number | null>(null);
  const { data: versionInfo } = useQuery({
    queryKey: ["apps", "catalogVersion"],
    queryFn: appsApi.catalogVersion,
  });

  const check = async () => {
    setChecking(true);
    try {
      const r: CatalogUpdateCheck = await appsApi.checkUpdate(source);
      setAvailable(r.updateAvailable ? r.latestVersion : null);
      if (r.updateAvailable) {
        toast.success(t("apps.updateAvailable", { version: r.latestVersion }));
      } else {
        toast.success(t("apps.upToDate"));
      }
    } catch (e) {
      toast.error(`${t("apps.checkFailed")}: ${errorMessage(e)}`);
    } finally {
      setChecking(false);
    }
  };

  const apply = async () => {
    setApplying(true);
    try {
      const meta: CatalogMeta = await appsApi.applyUpdate(source);
      setAvailable(null);
      toast.success(t("apps.updateApplied", { version: meta.version }));
    } catch (e) {
      toast.error(errorMessage(e));
    } finally {
      setApplying(false);
    }
  };

  return (
    <div className="flex flex-wrap items-center gap-2">
      {versionInfo && versionInfo[0] > 0 && (
        <span className="text-xs text-muted-foreground">
          {t("apps.catalogVersion")} v{versionInfo[0]}
          {versionInfo[1]?.source && versionInfo[1].source !== "builtin" && (
            <> · {versionInfo[1].source === "release" ? t("apps.sourceRelease") : t("apps.sourceMain")}</>
          )}
        </span>
      )}
      <Select
        id="appsUpdateSource"
        className="h-8 w-36 text-xs"
        value={source}
        onChange={(e) => {
          setSource(e.target.value as CatalogSource);
          setAvailable(null);
        }}
      >
        <option value="release">{t("apps.sourceRelease")}</option>
        <option value="main">{t("apps.sourceMain")}</option>
      </Select>
      {available !== null ? (
        <Button size="sm" onClick={apply} disabled={applying}>
          <ArrowUpCircle className="size-4" /> {t("apps.applyUpdate")}
        </Button>
      ) : (
        <Button size="sm" variant="outline" onClick={check} disabled={checking}>
          <RefreshCw className={checking ? "size-4 animate-spin" : "size-4"} /> {t("apps.checkUpdate")}
        </Button>
      )}
    </div>
  );
}

/** 应用卡片。 */
function AppCard(props: {
  entry: CatalogEntry;
  installed: boolean;
  installing: boolean;
  onInstall: () => void;
  onManage: () => void;
}) {
  const { t } = useTranslation();
  const { entry, installed, installing } = props;
  const spec = entry.spec;
  const { data: icon } = useQuery({
    queryKey: ["apps", "icon", spec.id, spec.icon],
    queryFn: () => appsApi.icon(spec.id, spec.icon || ""),
    enabled: !!spec.icon,
    staleTime: Infinity,
  });
  const { data: guide, } = useQuery({
    queryKey: ["apps", "guide", spec.id],
    queryFn: () => appsApi.guide(spec.id),
  });
  const [guideOpen, setGuideOpen] = useState(false);
  const iconUri = icon || null;

  return (
    <div className="flex flex-col gap-3 rounded-lg border border-border bg-card p-4">
      <div className="flex items-start gap-3">
        {iconUri ? (
          <img src={iconUri} alt="" className="size-10 shrink-0 rounded-lg" />
        ) : (
          <div className="flex size-10 shrink-0 items-center justify-center rounded-lg bg-muted">
            <Package className="size-5 text-muted-foreground" />
          </div>
        )}
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2">
            <span className="truncate font-medium">{spec.name}</span>
            <Badge variant="outline" className="text-[10px]">
              {entry.source === "builtin" ? t("apps.builtin") : t("apps.custom")}
            </Badge>
            {installing && <Badge className="text-[10px]">{t("apps.installing")}</Badge>}
            {installed && !installing && (
              <Badge variant="secondary" className="text-[10px]">{t("apps.installedOn")}</Badge>
            )}
          </div>
          <p className="mt-1 line-clamp-2 text-xs text-muted-foreground">{spec.description}</p>
        </div>
      </div>
      <div className="flex flex-wrap items-center gap-1">
        {(spec.tags ?? []).map((tag) => (
          <Badge key={tag} variant="outline" className="text-[10px]">{tag}</Badge>
        ))}
      </div>
      <div className="mt-auto flex items-center gap-2">
          {installed && !installing ? (
            <Button size="sm" className="flex-1" onClick={props.onManage}>
              <Settings2 className="size-4" /> {t("apps.manageAction")}
            </Button>
          ) : (
            /* 安装中也可点击：跳转会话附加到进行中的任务查看进度 */
            <Button size="sm" className="flex-1" onClick={props.onInstall}>
              {installing ? (
                <Loader2 className="size-4 animate-spin" />
              ) : (
                <Rocket className="size-4" />
              )}
              {installing ? t("apps.installing") : t("apps.installAction")}
            </Button>
          )}
        {guide && (
          <Button size="sm" variant="outline" onClick={() => setGuideOpen(true)}>
            <BookOpen className="size-4" /> {t("apps.viewGuide")}
          </Button>
        )}
        <button
          type="button"
          onClick={() => Browser.OpenURL(spec.website).catch((e) => toast.error(errorMessage(e)))}
          title={t("apps.website")}
          className="inline-flex size-8 items-center justify-center rounded-md text-muted-foreground hover:bg-accent hover:text-foreground"
        >
          <Globe className="size-4" />
        </button>
      </div>
      <Dialog open={guideOpen} onOpenChange={setGuideOpen}>
        <DialogContent className="max-h-[80vh] overflow-auto sm:max-w-2xl">
          <DialogHeader>
            <DialogTitle>{t("apps.guide")} · {spec.name}</DialogTitle>
          </DialogHeader>
          <pre className="whitespace-pre-wrap break-words text-xs leading-relaxed">{guide}</pre>
        </DialogContent>
      </Dialog>
    </div>
  );
}

/** 已安装总览（按主机分组，来自本地数据库记录）。 */
function InstalledOverview(props: { onOpen: (appId: string, hostId?: string) => void }) {
  const { t } = useTranslation();
  const { data: installations } = useQuery({
    queryKey: ["apps", "installations"],
    queryFn: () => appsApi.installations(""),
    refetchInterval: 30_000,
  });
  const { data: hosts } = useHosts();

  const byHost = useMemo(() => {
    const map = new Map<string, Installation[]>();
    for (const inst of installations ?? []) {
      const list = map.get(inst.hostId) ?? [];
      list.push(inst);
      map.set(inst.hostId, list);
    }
    return map;
  }, [installations]);

  if (!installations || installations.length === 0) {
    return (
      <div className="mt-8 rounded-lg border border-dashed border-border p-6 text-center text-sm text-muted-foreground">
        {t("apps.noInstallations")}
      </div>
    );
  }

  const hostName = (hostId: string) =>
    (hosts ?? []).find((h) => h.id === hostId)?.name ?? hostId.slice(0, 8);

  return (
    <div className="mt-8">
      <h2 className="mb-3 text-sm font-medium text-muted-foreground">{t("apps.allInstalled")}</h2>
      <div className="flex flex-col gap-2">
        {[...byHost.entries()].map(([hostId, list]) => (
          <div key={hostId} className="rounded-lg border border-border bg-card p-3">
            <div className="mb-2 text-xs font-medium text-muted-foreground">{hostName(hostId)}</div>
            <div className="flex flex-wrap gap-2">
              {list.map((inst) => (
                <button
                  key={inst.appId}
                  type="button"
                  onClick={() => props.onOpen(inst.appId, inst.hostId)}
                  className="flex items-center gap-2 rounded-md border border-border px-3 py-1.5 text-xs hover:bg-accent"
                >
                  <span
                    className={
                      "size-1.5 rounded-full " +
                      (inst.status === "installed"
                        ? "bg-emerald-500"
                        : inst.status === "installing"
                          ? "bg-amber-500"
                          : "bg-red-500")
                    }
                  />
                  {inst.appId}
                  <span className="text-muted-foreground">
                    {inst.status === "installed"
                      ? t("apps.installedOn")
                      : inst.status === "installing"
                        ? t("apps.installing")
                        : t("apps.installFailed")}
                  </span>
                </button>
              ))}
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}

/** 安装/管理入口：记录跳转意图 → 选主机 → 连接并跳转会话应用面板。 */
function useInstallOnHost() {
  const setPendingApp = useUIStore((s) => s.setPendingApp);
  const setView = useUIStore((s) => s.setView);
  const { data: hosts } = useHosts();
  const { activateOrConnect } = useActivateOrConnect();
  const [pending, setPending] = useState<string | null>(null);

  const jump = (appId: string, hostId: string) => {
    setPendingApp({ appId, hostId });
    setView("terminal");
    const host = (hosts ?? []).find((h) => h.id === hostId);
    if (host) activateOrConnect(host);
  };

  return {
    /** fixedHostId 给定时（已安装列表点进来）直接跳转，否则先选主机。 */
    open: (appId: string, fixedHostId?: string) => {
      if (fixedHostId && (hosts ?? []).some((h) => h.id === fixedHostId)) {
        jump(appId, fixedHostId);
        return;
      }
      setPending(appId);
    },
    dialog: (
      <HostPickerDialog
        open={pending !== null}
        onOpenChange={(v) => !v && setPending(null)}
        onPick={(hostId) => {
          if (pending) jump(pending, hostId);
          setPending(null);
        }}
      />
    ),
  };
}

/** 主机选择弹窗（安装流程第一步）。 */
function HostPickerDialog(props: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onPick: (hostId: string) => void;
}) {
  const { t } = useTranslation();
  const { data: hosts } = useHosts();
  const [picked, setPicked] = useState("");

  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{t("apps.selectHostTitle")}</DialogTitle>
        </DialogHeader>
        <p className="text-xs text-muted-foreground">{t("apps.selectHostHint")}</p>
        <div className="max-h-64 overflow-auto rounded-md border border-border">
          {(hosts ?? []).length === 0 && (
            <div className="p-4 text-center text-xs text-muted-foreground">{t("apps.noHosts")}</div>
          )}
          {(hosts ?? []).map((h) => (
            <label
              key={h.id}
              className="flex cursor-pointer items-center gap-2 border-b border-border px-3 py-2 text-sm last:border-0 hover:bg-accent"
            >
              <input
                type="radio"
                name="apps-target-host"
                checked={picked === h.id}
                onChange={() => setPicked(h.id)}
              />
              <span className="truncate">{h.name}</span>
              <span className="ml-auto truncate text-xs text-muted-foreground">
                {h.host}:{h.port}
              </span>
            </label>
          ))}
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => props.onOpenChange(false)}>
            {t("apps.cancel")}
          </Button>
          <Button onClick={() => props.onPick(picked)} disabled={!picked}>
            {t("apps.installAction")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
