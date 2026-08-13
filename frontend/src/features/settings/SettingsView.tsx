import { useEffect, useState } from "react";
import { Check, Languages } from "lucide-react";
import { useTranslation } from "react-i18next";

import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import {
  TERMINAL_THEMES,
  getTerminalTheme,
} from "@/features/terminal/themes";
import { useTerminalStore } from "@/features/terminal/terminal.store";
import { ConfigService } from "@/../bindings/github.com/ai-remote/workspace/internal/interfaces";
import {
  SecurityMode,
  type AppConfig,
} from "@/../bindings/github.com/ai-remote/workspace/internal/domain/models";
import { cn } from "@/lib/utils";

const DEFAULT_CONFIG: AppConfig = {
  securityMode: SecurityMode.SecurityBalanced,
  defaultShell: "/bin/bash",
  theme: "dark",
  terminalTheme: "cobalt2",
  llm: { baseUrl: "https://api.openai.com/v1", model: "gpt-4o" },
};

/**
 * Settings view. Phase 1 read-only fields; terminal theme is editable and
 * persisted via ConfigService.SetAppConfig.
 */
export function SettingsView() {
  const { t, i18n } = useTranslation();
  const [config, setConfig] = useState<AppConfig>(DEFAULT_CONFIG);
  const [loaded, setLoaded] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [savingTheme, setSavingTheme] = useState<string | null>(null);
  const setTerminalThemeId = useTerminalStore((s) => s.setThemeId);

  useEffect(() => {
    let cancelled = false;
    ConfigService.GetAppConfig()
      .then((res: AppConfig) => {
        if (!cancelled) {
          // terminalTheme may be absent on an older config; fall back.
          const themeId = res.terminalTheme || "cobalt2";
          setConfig({ ...res, terminalTheme: themeId });
          setTerminalThemeId(themeId); // sync the terminal store on load
          setLoaded(true);
        }
      })
      .catch((err: unknown) => {
        if (!cancelled) {
          setError(err instanceof Error ? err.message : String(err));
          setLoaded(true);
        }
      });
    return () => {
      cancelled = true;
    };
  }, []);

  const selectTheme = async (themeId: string) => {
    const prev = config;
    const next = { ...config, terminalTheme: themeId };
    setConfig(next);
    setTerminalThemeId(themeId); // live-update open terminals
    setSavingTheme(themeId);
    try {
      await ConfigService.SetAppConfig(next);
    } catch (e) {
      // Revert on failure.
      setConfig(prev);
      setTerminalThemeId(prev.terminalTheme);
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setSavingTheme(null);
    }
  };

  const activeTheme = getTerminalTheme(config.terminalTheme);

  return (
    <div className="mx-auto max-w-2xl p-8">
      <h1 className="text-xl font-semibold tracking-tight">{t("settings.title")}</h1>
      <p className="mt-1 text-sm text-muted-foreground">
        Application configuration, stored locally.
      </p>

      <Card className="mt-6">
        <CardHeader>
          <CardTitle className="text-base">{t("settings.securityMode")}</CardTitle>
          <CardDescription>{t("settings.securityModeDesc")}</CardDescription>
        </CardHeader>
        <CardContent>
          <span className="inline-flex rounded-full border border-border bg-muted px-3 py-1 text-sm">
            {t(`security.${config.securityMode}`)}
          </span>
        </CardContent>
      </Card>

      {/* Terminal colour scheme */}
      <Card className="mt-4">
        <CardHeader>
          <CardTitle className="text-base">{t("settings.terminalScheme")}</CardTitle>
          <CardDescription>{t("settings.terminalSchemeDesc")}</CardDescription>
        </CardHeader>
        <CardContent>
          <div className="grid grid-cols-2 gap-3 sm:grid-cols-3">
            {TERMINAL_THEMES.map((def) => {
              const selected = config.terminalTheme === def.id;
              return (
                <button
                  key={def.id}
                  type="button"
                  onClick={() => selectTheme(def.id)}
                  disabled={savingTheme !== null}
                  className={cn(
                    "group relative flex flex-col gap-2 rounded-[var(--radius)] border p-2 text-left transition-colors",
                    selected
                      ? "border-primary ring-1 ring-primary"
                      : "border-border hover:border-primary/40",
                  )}
                >
                  {/* Preview swatch using the theme's ANSI palette. */}
                  <div
                    className="flex h-12 items-center justify-center gap-1 rounded-[calc(var(--radius)-2px)] font-mono text-[10px] font-bold"
                    style={{ background: def.theme.background }}
                  >
                    <span style={{ color: def.theme.red }}>●</span>
                    <span style={{ color: def.theme.green }}>●</span>
                    <span style={{ color: def.theme.yellow }}>●</span>
                    <span style={{ color: def.theme.blue }}>●</span>
                    <span style={{ color: def.theme.magenta }}>●</span>
                    <span style={{ color: def.theme.cyan }}>●</span>
                  </div>
                  <div className="flex items-center justify-between">
                    <span className="text-xs font-medium">{def.label}</span>
                    {selected && (
                      <Check className="h-3.5 w-3.5 text-primary" />
                    )}
                  </div>
                </button>
              );
            })}
          </div>
          <p className="mt-3 text-xs text-muted-foreground">
            {t("settings.selected")}: <span className="font-medium">{activeTheme.label}</span>
            {activeTheme.light && " (light)"}
          </p>
        </CardContent>
      </Card>

      {/* Language */}
      <Card className="mt-4">
        <CardHeader>
          <CardTitle className="flex items-center gap-2 text-base">
            <Languages className="h-4 w-4 text-primary" />
            {t("settings.language")}
          </CardTitle>
          <CardDescription>{t("settings.languageDesc")}</CardDescription>
        </CardHeader>
        <CardContent>
          <div className="flex gap-2">
            <button
              type="button"
              onClick={() => i18n.changeLanguage("zh")}
              className={cn(
                "rounded-[var(--radius)] border px-3 py-1.5 text-sm transition-colors",
                i18n.language?.startsWith("zh")
                  ? "border-primary text-primary"
                  : "border-border text-muted-foreground hover:border-primary/40",
              )}
            >
              {t("settings.chinese")}
            </button>
            <button
              type="button"
              onClick={() => i18n.changeLanguage("en")}
              className={cn(
                "rounded-[var(--radius)] border px-3 py-1.5 text-sm transition-colors",
                i18n.language?.startsWith("en")
                  ? "border-primary text-primary"
                  : "border-border text-muted-foreground hover:border-primary/40",
              )}
            >
              {t("settings.english")}
            </button>
          </div>
        </CardContent>
      </Card>

      <Card className="mt-4">
        <CardHeader>
          <CardTitle className="text-base">{t("settings.runtime")}</CardTitle>
          <CardDescription>{t("settings.runtimeDesc")}</CardDescription>
        </CardHeader>
        <CardContent>
          <dl className="grid grid-cols-2 gap-y-2 text-sm">
            <dt className="text-muted-foreground">{t("settings.defaultShell")}</dt>
            <dd className="font-mono">{config.defaultShell}</dd>
            <dt className="text-muted-foreground">Theme</dt>
            <dd>{config.theme}</dd>
          </dl>
        </CardContent>
      </Card>

      <p className="mt-6 text-xs text-muted-foreground">
        {error
          ? t("settings.loadError", { error })
          : loaded
            ? t("settings.loaded")
            : t("common.loading")}
      </p>
    </div>
  );
}
