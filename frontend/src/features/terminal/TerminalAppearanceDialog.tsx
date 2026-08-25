import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Palette, Minus, Plus } from "lucide-react";

import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Select } from "@/components/ui/select";
import { cn } from "@/lib/utils";

import { TERMINAL_THEMES, getTerminalTheme } from "@/features/terminal/themes";
import { TERMINAL_FONTS, terminalFontFamily } from "@/features/terminal/fonts";

const MIN_FONT_SIZE = 8;
const MAX_FONT_SIZE = 32;

const clampSize = (n: number) =>
  Math.min(MAX_FONT_SIZE, Math.max(MIN_FONT_SIZE, n));

interface TerminalAppearanceDialogProps {
  open: boolean;
  /** Theme id currently in effect for the pane (pre-selected in the grid). */
  currentThemeId: string;
  /** Font family value in effect ("" = built-in default stack). */
  currentFont: string;
  /** Effective font size in px (base size + live zoom). */
  currentFontSize: number;
  /** Called with the chosen theme id, font value and font size on confirm. */
  onConfirm: (themeId: string, font: string, fontSize: number) => void;
  onClose: () => void;
}

/**
 * Terminal appearance picker opened from the terminal context menu: colour
 * scheme, font family and font size for the current pane. Every control
 * previews live inside the dialog (mock terminal sample rendered with the
 * selected palette/font); nothing is applied to the actual pane until the
 * user hits Confirm.
 */
export function TerminalAppearanceDialog({
  open,
  currentThemeId,
  currentFont,
  currentFontSize,
  onConfirm,
  onClose,
}: TerminalAppearanceDialogProps) {
  const { t } = useTranslation();
  const [themeId, setThemeId] = useState(currentThemeId);
  const [font, setFont] = useState(currentFont);
  const [fontSize, setFontSize] = useState(() => clampSize(currentFontSize));

  // Re-sync the selections each time the dialog opens.
  useEffect(() => {
    if (open) {
      setThemeId(currentThemeId);
      setFont(currentFont);
      setFontSize(clampSize(currentFontSize));
    }
  }, [open, currentThemeId, currentFont, currentFontSize]);

  const theme = getTerminalTheme(themeId);

  return (
    <Dialog open={open} onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2 text-base">
            <Palette className="h-4 w-4 text-primary" />
            {t("termMenu.terminalAppearance")}
          </DialogTitle>
          <DialogDescription>{t("termAppearanceDialog.desc")}</DialogDescription>
        </DialogHeader>

        <div className="flex flex-col gap-4">
          {/* Colour scheme grid — swatch chips beat a long dropdown once the
              catalogue grew past a handful of schemes. */}
          <div className="flex flex-col gap-1.5">
            <span className="text-sm text-muted-foreground">
              {t("termAppearanceDialog.scheme")}
            </span>
            <div className="grid max-h-44 grid-cols-2 gap-1.5 overflow-y-auto pr-1">
              {TERMINAL_THEMES.map((def) => {
                const active = def.id === themeId;
                return (
                  <button
                    key={def.id}
                    type="button"
                    onClick={() => setThemeId(def.id)}
                    className={cn(
                      "flex items-center gap-2 rounded-[calc(var(--radius)-2px)] border px-2 py-1.5 text-left text-xs transition-colors",
                      active
                        ? "border-primary bg-primary/10 text-primary"
                        : "border-border text-foreground hover:bg-accent/50",
                    )}
                  >
                    <span
                      className="flex h-5 shrink-0 items-center gap-0.5 rounded-[3px] border border-black/20 px-0.5"
                      style={{ background: def.theme.background }}
                    >
                      {(["red", "green", "yellow", "blue", "cyan"] as const).map((c) => (
                        <span
                          key={c}
                          className="h-3 w-1.5 rounded-[1px]"
                          style={{ background: def.theme[c] }}
                        />
                      ))}
                    </span>
                    <span className="truncate">{def.label}</span>
                  </button>
                );
              })}
            </div>
          </div>

          {/* Font family */}
          <div className="grid grid-cols-[5rem_1fr] items-center gap-3">
            <span className="text-sm text-muted-foreground">
              {t("termAppearanceDialog.font")}
            </span>
            <Select value={font} onChange={(e) => setFont(e.target.value)}>
              {TERMINAL_FONTS.map((f) => (
                <option key={f.value} value={f.value}>
                  {f.label}
                </option>
              ))}
            </Select>
          </div>

          {/* Font size stepper */}
          <div className="grid grid-cols-[5rem_1fr] items-center gap-3">
            <span className="text-sm text-muted-foreground">
              {t("termAppearanceDialog.fontSize")}
            </span>
            <div className="flex items-center gap-2">
              <Button
                variant="outline"
                size="icon"
                className="h-7 w-7"
                disabled={fontSize <= MIN_FONT_SIZE}
                onClick={() => setFontSize((n) => clampSize(n - 1))}
                aria-label={t("termAppearanceDialog.smaller")}
              >
                <Minus className="h-3.5 w-3.5" />
              </Button>
              <span className="w-12 text-center font-mono text-sm">{fontSize}px</span>
              <Button
                variant="outline"
                size="icon"
                className="h-7 w-7"
                disabled={fontSize >= MAX_FONT_SIZE}
                onClick={() => setFontSize((n) => clampSize(n + 1))}
                aria-label={t("termAppearanceDialog.larger")}
              >
                <Plus className="h-3.5 w-3.5" />
              </Button>
            </div>
          </div>

          {/* Live preview — a mock terminal rendered with the selected
              palette, font family and size. */}
          <div
            className="overflow-hidden rounded-[var(--radius)] border border-border font-mono leading-relaxed"
            style={{
              background: theme.theme.background,
              fontFamily: terminalFontFamily(font),
              fontSize: `${fontSize}px`,
            }}
          >
            <div
              className="border-b border-white/10 px-3 py-1 text-[11px]"
              style={{ color: theme.theme.brightBlack }}
            >
              {t("termAppearanceDialog.preview")}
            </div>
            <div className="space-y-0.5 px-3 py-2.5">
              <div>
                <span style={{ color: theme.theme.green }}>user@web-prod</span>
                <span style={{ color: theme.theme.foreground }}>:~$ </span>
                <span style={{ color: theme.theme.brightYellow }}>ls</span>
                <span style={{ color: theme.theme.foreground }}> -la</span>
              </div>
              <div>
                <span style={{ color: theme.theme.blue }}>drwxr-xr-x</span>
                <span style={{ color: theme.theme.foreground }}>  .ssh</span>
              </div>
              <div>
                <span style={{ color: theme.theme.brightBlack }}>-rw-r--r--</span>
                <span style={{ color: theme.theme.foreground }}>  deploy.log</span>
              </div>
              <div>
                <span style={{ color: theme.theme.green }}>user@web-prod</span>
                <span style={{ color: theme.theme.foreground }}>:~$ </span>
                <span style={{ color: theme.theme.brightYellow }}>tail</span>
                <span style={{ color: theme.theme.foreground }}> deploy.log</span>
              </div>
              <div style={{ color: theme.theme.green }}>[INFO] service started on :8080</div>
              <div style={{ color: theme.theme.yellow }}>[WARN] disk usage at 82%</div>
              <div style={{ color: theme.theme.red }}>[ERROR] upstream timeout</div>
              <div>
                <span style={{ color: theme.theme.green }}>user@web-prod</span>
                <span style={{ color: theme.theme.foreground }}>:~$ </span>
                <span
                  className="inline-block h-[1em] w-[0.55em] translate-y-[0.15em] animate-pulse"
                  style={{ background: theme.theme.cursor }}
                />
              </div>
              {/* 16-colour swatch strip */}
              <div className="flex flex-wrap gap-1 pt-1.5">
                {(
                  [
                    "black", "red", "green", "yellow",
                    "blue", "magenta", "cyan", "white",
                    "brightBlack", "brightRed", "brightGreen", "brightYellow",
                    "brightBlue", "brightMagenta", "brightCyan", "brightWhite",
                  ] as const
                ).map((c) => (
                  <span
                    key={c}
                    title={c}
                    className="h-4 w-4 rounded-[3px] border border-black/20"
                    style={{ background: theme.theme[c] }}
                  />
                ))}
              </div>
            </div>
          </div>
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={onClose}>
            {t("common.cancel")}
          </Button>
          <Button
            className="bg-primary text-primary-foreground"
            onClick={() => onConfirm(themeId, font, fontSize)}
          >
            {t("common.confirm")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
