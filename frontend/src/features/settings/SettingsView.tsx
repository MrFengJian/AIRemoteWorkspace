import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import {
  Palette,
  Languages,
  Settings2,
  Info,
  Bot,
  BrainCircuit,
  Check,
  Keyboard,
  Sun,
  Moon,
  Monitor,
} from "lucide-react";

import { Label } from "@/components/ui/label";
import { Select } from "@/components/ui/select";
import { Badge } from "@/components/ui/badge";
import { Input } from "@/components/ui/input";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { ConfigService, SystemService } from "@/../bindings/github.com/ai-remote/workspace/internal/interfaces";
import {
  SecurityMode,
  type AppConfig,
} from "@/../bindings/github.com/ai-remote/workspace/internal/domain/models";
import { applyTheme, applyFonts } from "@/app/providers/ThemeProvider";
import { useUIStore, type SettingsCategory } from "@/stores/ui.store";
import { ModelSettingsSection } from "@/features/settings/ModelSettingsSection";
import { ShortcutSettingsSection } from "@/features/settings/ShortcutSettingsSection";
import { AgentSettingsSection } from "@/features/settings/AgentSettingsSection";
import { cn } from "@/lib/utils";
import { toast, errorMessage } from "@/lib/toast";

const DEFAULT_CONFIG: AppConfig = {
  securityMode: SecurityMode.SecurityBalanced,
  defaultShell: "/bin/bash",
  theme: "dark",
  uiFont: "",
  fontSize: 13,
  cjkFont: "",
  terminalTheme: "",
  terminalFont: "",
  terminalFontSize: 0,
  middleClickAction: "pasteSelection",
  monitorIntervalSeconds: 60,
  agent: {
    maxSteps: 100,
    historyTurns: 40,
    toolOutputLimitKB: 64,
    customInstructions: "",
  },
  transfer: {
    chunkKb: 256,
    maxUploadMb: 4096,
    maxDownloadMb: 4096,
  },
  llm: { baseUrl: "https://api.openai.com/v1", model: "gpt-4o" },
};

/** Common sans-serif font options. Terminal font/size live per-host (host
 *  edit dialog → 外观 tab), Xshell-style — not here. */
const UI_FONTS = [
  { value: "", label: "System Default" },
  { value: "Inter", label: "Inter" },
  { value: "Segoe UI", label: "Segoe UI" },
  { value: "system-ui", label: "system-ui" },
  { value: "SF Pro Text", label: "SF Pro Text" },
  { value: "Microsoft YaHei", label: "Microsoft YaHei" },
  { value: "Helvetica Neue", label: "Helvetica Neue" },
  { value: "Arial", label: "Arial" },
];

/** Common CJK font options. */
const CJK_FONTS = [
  { value: "", label: "Follow UI font" },
  { value: "Microsoft YaHei", label: "Microsoft YaHei" },
  { value: "PingFang SC", label: "PingFang SC" },
  { value: "Noto Sans CJK SC", label: "Noto Sans CJK SC" },
  { value: "Source Han Sans SC", label: "Source Han Sans SC" },
  { value: "SimHei", label: "SimHei" },
  { value: "WenQuanYi Micro Hei", label: "WenQuanYi Micro Hei" },
];

export function SettingsView() {
  const { t, i18n } = useTranslation();
  const [config, setConfig] = useState<AppConfig>(DEFAULT_CONFIG);
  // Category lives in the global UI store so other views can deep-link into a
  // specific settings section (e.g. the agent's "open model settings").
  const category = useUIStore((s) => s.settingsCategory);
  const setCategory = useUIStore((s) => s.setSettingsCategory);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    let cancelled = false;
    ConfigService.GetAppConfig()
      .then((res: AppConfig) => {
        if (!cancelled) setConfig(res);
      })
      .catch(() => {});
    return () => {
      cancelled = true;
    };
  }, []);

  /** Persist config changes to backend + apply live effects. Returns whether
   *  the save succeeded (callers with extra runtime state can roll back). */
  const updateConfig = async (patch: Partial<AppConfig>): Promise<boolean> => {
    const next = { ...config, ...patch };
    setConfig(next);
    // Apply live effects immediately.
    if (patch.theme) applyTheme(patch.theme);
    if (patch.uiFont !== undefined || patch.cjkFont !== undefined || patch.fontSize) {
      applyFonts(next.uiFont, next.cjkFont, next.fontSize);
    }
    // Persist.
    setSaving(true);
    try {
      await ConfigService.SetAppConfig(next);
      return true;
    } catch (e) {
      // Revert and tell the user — otherwise the change looks saved but isn't.
      setConfig(config);
      if (patch.theme) applyTheme(config.theme || "dark");
      if (patch.uiFont !== undefined || patch.cjkFont !== undefined || patch.fontSize) {
        applyFonts(config.uiFont, config.cjkFont, config.fontSize);
      }
      toast.error(`${t("settings.saveFailed")}: ${errorMessage(e)}`);
      return false;
    } finally {
      setSaving(false);
    }
  };

  const categories: { id: SettingsCategory; label: string; icon: typeof Palette }[] = [
    { id: "appearance", label: t("settings.appearance"), icon: Palette },
    { id: "language", label: t("settings.language"), icon: Languages },
    { id: "models", label: t("settings.models.title"), icon: Bot },
    { id: "agent", label: t("settings.agentTitle"), icon: BrainCircuit },
    { id: "shortcuts", label: t("settings.shortcuts"), icon: Keyboard },
    { id: "advanced", label: t("settings.advanced"), icon: Settings2 },
    { id: "about", label: t("settings.about"), icon: Info },
  ];

  return (
    <div className="flex h-full overflow-hidden">
      {/* Left sidebar: category navigation */}
      <nav className="flex w-48 shrink-0 flex-col gap-0.5 border-r border-border bg-card p-2">
        {categories.map((cat) => {
          const Icon = cat.icon;
          return (
            <button
              key={cat.id}
              type="button"
              onClick={() => setCategory(cat.id)}
              className={cn(
                "flex items-center gap-2 rounded-[var(--radius)] px-3 py-2 text-sm transition-colors",
                category === cat.id
                  ? "bg-accent text-foreground"
                  : "text-muted-foreground hover:bg-accent/50 hover:text-foreground",
              )}
            >
              <Icon className="h-4 w-4 shrink-0" />
              {cat.label}
            </button>
          );
        })}
      </nav>

      {/* Right: content area */}
      <div className="min-w-0 flex-1 overflow-auto p-6">
        <div className="mx-auto max-w-2xl">
          {category === "appearance" && <AppearanceSection config={config} update={updateConfig} saving={saving} />}
          {category === "language" && <LanguageSection i18n={i18n} />}
          {category === "models" && <ModelSettingsSection />}
          {category === "agent" && <AgentSettingsSection config={config} update={updateConfig} />}
          {category === "shortcuts" && <ShortcutSettingsSection config={config} update={updateConfig} />}
          {category === "advanced" && <AdvancedSection config={config} update={updateConfig} />}
          {category === "about" && <AboutSection />}
        </div>
      </div>
    </div>
  );
}

// ── Appearance ───────────────────────────────────────────────────────

function AppearanceSection({
  config,
  update,
  saving,
}: {
  config: AppConfig;
  update: (patch: Partial<AppConfig>) => void;
  saving: boolean;
}) {
  const { t } = useTranslation();
  const themeOptions = [
    { value: "light", label: t("settings.themeLight"), icon: Sun },
    { value: "dark", label: t("settings.themeDark"), icon: Moon },
    { value: "auto", label: t("settings.themeAuto"), icon: Monitor },
  ];

  return (
    <div className="flex flex-col gap-4">
      <h2 className="text-lg font-semibold">{t("settings.appearance")}</h2>

      {/* Theme mode */}
      <Card>
        <CardHeader>
          <CardTitle className="text-sm">{t("settings.themeMode")}</CardTitle>
          <CardDescription>{t("settings.themeModeDesc")}</CardDescription>
        </CardHeader>
        <CardContent>
          <div className="flex gap-2">
            {themeOptions.map((opt) => {
              const Icon = opt.icon;
              const active = (config.theme || "dark") === opt.value;
              return (
                <button
                  key={opt.value}
                  type="button"
                  onClick={() => update({ theme: opt.value })}
                  className={cn(
                    "flex flex-1 flex-col items-center gap-1.5 rounded-[var(--radius)] border p-3 transition-colors",
                    active
                      ? "border-primary bg-primary/10 text-primary"
                      : "border-border text-muted-foreground hover:border-primary/40",
                  )}
                >
                  <Icon className="h-5 w-5" />
                  <span className="text-xs font-medium">{opt.label}</span>
                  {active && <Check className="h-3 w-3" />}
                </button>
              );
            })}
          </div>
        </CardContent>
      </Card>

      {/* Font settings */}
      <Card>
        <CardHeader>
          <CardTitle className="text-sm">{t("settings.fontTitle")}</CardTitle>
          <CardDescription>{t("settings.fontDesc")}</CardDescription>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          {/* UI font */}
          <div className="grid grid-cols-[8rem_1fr] items-center gap-3">
            <Label htmlFor="uiFont">{t("settings.uiFont")}</Label>
            <Select
              id="uiFont"
              value={config.uiFont}
              onChange={(e) => update({ uiFont: e.target.value })}
            >
              {UI_FONTS.map((f) => (
                <option key={f.value} value={f.value}>{f.label}</option>
              ))}
            </Select>
          </div>

          {/* CJK font */}
          <div className="grid grid-cols-[8rem_1fr] items-center gap-3">
            <Label htmlFor="cjkFont">{t("settings.cjkFont")}</Label>
            <Select
              id="cjkFont"
              value={config.cjkFont}
              onChange={(e) => update({ cjkFont: e.target.value })}
            >
              {CJK_FONTS.map((f) => (
                <option key={f.value} value={f.value}>{f.label}</option>
              ))}
            </Select>
          </div>

          {/* Font size */}
          <div className="grid grid-cols-[8rem_1fr] items-center gap-3">
            <Label htmlFor="fontSize">{t("settings.fontSize")}</Label>
            <div className="flex items-center gap-3">
              <input
                type="range"
                min={11}
                max={20}
                value={config.fontSize || 13}
                onChange={(e) => update({ fontSize: Number(e.target.value) })}
                className="flex-1 accent-primary"
              />
              <span className="w-10 text-center text-sm font-mono">
                {config.fontSize || 13}px
              </span>
            </div>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}

// ── Language ─────────────────────────────────────────────────────────

function LanguageSection({ i18n }: { i18n: ReturnType<typeof useTranslation>["i18n"] }) {
  const { t } = useTranslation();
  return (
    <div className="flex flex-col gap-4">
      <h2 className="text-lg font-semibold">{t("settings.language")}</h2>
      <Card>
        <CardHeader>
          <CardTitle className="text-sm">{t("settings.interfaceLanguage")}</CardTitle>
          <CardDescription>{t("settings.languageDesc")}</CardDescription>
        </CardHeader>
        <CardContent>
          <div className="grid grid-cols-[8rem_1fr] items-center gap-3">
            <Label>{t("settings.language")}</Label>
            <Select
              value={i18n.language?.split("-")[0] ?? "en"}
              onChange={(e) => i18n.changeLanguage(e.target.value)}
              className="max-w-xs"
            >
              <option value="en">English</option>
              <option value="zh">简体中文</option>
            </Select>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}

// ── Advanced ─────────────────────────────────────────────────────────

const MONITOR_INTERVALS = [15, 30, 60, 120, 300, 600];

function AdvancedSection({
  config,
  update,
}: {
  config: AppConfig;
  update: (patch: Partial<AppConfig>) => Promise<boolean>;
}) {
  const { t } = useTranslation();
  const interval = config.monitorIntervalSeconds || 60;
  return (
    <div className="flex flex-col gap-4">
      <h2 className="text-lg font-semibold">{t("settings.advanced")}</h2>
      <Card>
        <CardHeader>
          <CardTitle className="text-sm">{t("settings.monitorTitle")}</CardTitle>
          <CardDescription>{t("settings.monitorDesc")}</CardDescription>
        </CardHeader>
        <CardContent>
          <div className="grid grid-cols-[8rem_1fr] items-center gap-3">
            <Label htmlFor="monitorInterval">{t("settings.monitorInterval")}</Label>
            <Select
              id="monitorInterval"
              value={String(interval)}
              onChange={(e) => void update({ monitorIntervalSeconds: Number(e.target.value) })}
              className="max-w-40"
            >
              {MONITOR_INTERVALS.map((s) => (
                <option key={s} value={s}>
                  {s >= 60 ? `${s / 60} ${t("settings.min")}` : `${s} ${t("settings.sec")}`}
                </option>
              ))}
            </Select>
          </div>
        </CardContent>
      </Card>
      <Card>
        <CardHeader>
          <CardTitle className="text-sm">{t("settings.transferTitle")}</CardTitle>
          <CardDescription>{t("settings.transferDesc")}</CardDescription>
        </CardHeader>
        <CardContent className="flex flex-col gap-3">
          {(
            [
              {
                id: "transferChunk",
                key: "chunkKb" as const,
                label: t("settings.transferChunk"),
                hint: t("settings.transferChunkHint"),
                min: 64,
                max: 4096,
              },
              {
                id: "transferUpload",
                key: "maxUploadMb" as const,
                label: t("settings.transferUploadLimit"),
                hint: t("settings.transferLimitHint"),
                min: 1,
                max: 1048576,
              },
              {
                id: "transferDownload",
                key: "maxDownloadMb" as const,
                label: t("settings.transferDownloadLimit"),
                hint: t("settings.transferLimitHint"),
                min: 1,
                max: 1048576,
              },
            ] as const
          ).map((f) => (
            <div key={f.id} className="grid grid-cols-[10rem_9rem_1fr] items-center gap-3">
              <Label htmlFor={f.id}>{f.label}</Label>
              <Input
                id={f.id}
                type="number"
                min={f.min}
                max={f.max}
                value={config.transfer?.[f.key] ?? 256}
                onChange={(e) => {
                  const n = Math.round(Number(e.target.value));
                  if (!Number.isFinite(n)) return;
                  void update({
                    transfer: { ...config.transfer, [f.key]: Math.min(f.max, Math.max(f.min, n)) },
                  });
                }}
              />
              <p className="text-xs text-muted-foreground">{f.hint}</p>
            </div>
          ))}
        </CardContent>
      </Card>
      <Card>
        <CardHeader>
          <CardTitle className="text-sm">{t("settings.securityMode")}</CardTitle>
          <CardDescription>{t("settings.securityModeDesc")}</CardDescription>
        </CardHeader>
        <CardContent>
          <Badge variant="secondary">
            {t(`security.${config.securityMode}`)}
          </Badge>
        </CardContent>
      </Card>
      <Card>
        <CardHeader>
          <CardTitle className="text-sm">{t("settings.runtime")}</CardTitle>
          <CardDescription>{t("settings.runtimeDesc")}</CardDescription>
        </CardHeader>
        <CardContent>
          <dl className="grid grid-cols-2 gap-y-2 text-sm">
            <dt className="text-muted-foreground">{t("settings.defaultShell")}</dt>
            <dd className="font-mono">{config.defaultShell}</dd>
          </dl>
        </CardContent>
      </Card>
    </div>
  );
}

// ── About ────────────────────────────────────────────────────────────

function AboutSection() {
  const { t } = useTranslation();
  const [info, setInfo] = useState<{
    appName: string;
    version: string;
    platform: string;
    goVersion: string;
  } | null>(null);

  useEffect(() => {
    SystemService.SystemInfo()
      .then(setInfo)
      .catch(() => {});
  }, []);

  return (
    <div className="flex flex-col gap-4">
      <h2 className="text-lg font-semibold">{t("settings.about")}</h2>
      <Card>
        <CardContent className="pt-6">
          <dl className="grid grid-cols-[8rem_1fr] gap-y-3 text-sm">
            <dt className="text-muted-foreground">{t("settings.appName")}</dt>
            <dd className="font-medium">{info?.appName ?? "AI Remote Workspace"}</dd>
            <dt className="text-muted-foreground">{t("settings.version")}</dt>
            <dd className="font-mono">{info?.version ?? "—"}</dd>
            <dt className="text-muted-foreground">{t("settings.platform")}</dt>
            <dd className="font-mono">{info?.platform ?? "—"}</dd>
            <dt className="text-muted-foreground">{t("settings.goVersion")}</dt>
            <dd className="font-mono">{info?.goVersion ?? "—"}</dd>
          </dl>
        </CardContent>
      </Card>
    </div>
  );
}
