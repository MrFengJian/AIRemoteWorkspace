import { useEffect, type ReactNode } from "react";
import { Events } from "@wailsio/runtime";
import i18n from "i18next";

import { ConfigService } from "@/../bindings/github.com/ai-remote/workspace/internal/interfaces";

interface ThemeProviderProps {
  children: ReactNode;
}

/**
 * ThemeProvider applies the persisted AppConfig theme and font settings to
 * the document. It reads AppConfig on startup AND whenever the backend
 * broadcasts "config:changed" — settings saved in any window (the standalone
 * SFTP window included) re-apply everywhere, since each window is a separate
 * webview with its own DOM.
 */
export function ThemeProvider({ children }: ThemeProviderProps) {
  useEffect(() => {
    const apply = () => {
      ConfigService.GetAppConfig()
        .then((cfg) => {
          applyTheme(cfg.theme || "dark");
          applyFonts(cfg.uiFont, cfg.cjkFont, cfg.fontSize || 13);
          syncLanguage();
        })
        .catch(() => {
          // defaults: dark theme, system fonts, 13px
          applyTheme("dark");
        });
    };
    void apply();
    const off = Events.On("config:changed", apply);
    return () => {
      off();
    };
  }, []);

  return <>{children}</>;
}

/**
 * Language is persisted in localStorage (the i18next detector cache), which
 * the windows of the app share. When another window switched the language,
 * this window's i18n instance only finds out here — re-align with storage.
 */
function syncLanguage() {
  const stored = localStorage.getItem("lang")?.split("-")[0];
  if (stored && stored !== i18n.language?.split("-")[0]) {
    void i18n.changeLanguage(stored);
  }
}

/** Apply the colour-scheme mode to the document root. */
export function applyTheme(mode: string) {
  const valid = mode === "light" || mode === "dark" || mode === "auto" ? mode : "dark";
  document.documentElement.setAttribute("data-theme", valid);
}

/** Apply font family + size via CSS custom properties. */
export function applyFonts(uiFont: string, cjkFont: string, fontSize: number) {
  const root = document.documentElement.style;
  // Compose the FULL family stack here. The old scheme injected an empty
  // string into the CSS font-family list when a font was unset — an empty
  // quoted name is invalid and silently voided the entire declaration, so
  // the setting never applied. Empty value = property removed = the
  // stylesheet default (--font-sans) takes over again.
  const families = [uiFont, cjkFont]
    .map((f) => f.trim())
    .filter((f) => f.length > 0)
    .map((f) => `"${f.replace(/\\/g, "\\\\").replace(/"/g, '\\"')}"`);
  root.setProperty("--app-font-stack", families.join(", "));
  root.setProperty("--app-font-size", `${fontSize}px`);
}
