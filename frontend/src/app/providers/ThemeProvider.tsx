import { useEffect, type ReactNode } from "react";

import { ConfigService } from "@/../bindings/github.com/ai-remote/workspace/internal/interfaces";

interface ThemeProviderProps {
  children: ReactNode;
}

/**
 * ThemeProvider applies the persisted AppConfig theme and font settings to the
 * document on app startup. It reads AppConfig once and sets:
 *
 *   - <html data-theme="light|dark|auto"> for CSS variable switching
 *   --app-font / --cjk-font / --app-font-size CSS variables on :root
 *
 * SettingsView updates these live when the user changes a setting.
 */
export function ThemeProvider({ children }: ThemeProviderProps) {
  useEffect(() => {
    ConfigService.GetAppConfig()
      .then((cfg) => {
        applyTheme(cfg.theme || "dark");
        applyFonts(cfg.uiFont, cfg.cjkFont, cfg.fontSize || 13);
      })
      .catch(() => {
        // defaults: dark theme, system fonts, 13px
        applyTheme("dark");
      });
  }, []);

  return <>{children}</>;
}

/** Apply the colour-scheme mode to the document root. */
export function applyTheme(mode: string) {
  const valid = mode === "light" || mode === "dark" || mode === "auto" ? mode : "dark";
  document.documentElement.setAttribute("data-theme", valid);
}

/** Apply font family + size via CSS custom properties. */
export function applyFonts(uiFont: string, cjkFont: string, fontSize: number) {
  const root = document.documentElement.style;
  root.setProperty("--app-font", uiFont || "var(--font-sans)");
  root.setProperty("--cjk-font", cjkFont || "");
  root.setProperty("--app-font-size", `${fontSize}px`);
}
