import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";

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
import { Palette } from "lucide-react";

import { TERMINAL_THEMES, getTerminalTheme } from "@/features/terminal/themes";

interface TerminalThemeDialogProps {
  open: boolean;
  /** Theme id currently in effect for the pane (pre-selected in the dropdown). */
  currentThemeId: string;
  /** Called with the chosen theme id when the user confirms. */
  onConfirm: (themeId: string) => void;
  onClose: () => void;
}

/**
 * Terminal colour-scheme picker opened from the terminal context menu.
 * The dropdown selection previews live inside the dialog (mock terminal
 * sample rendered with the scheme's palette); nothing is applied to the
 * actual pane until the user hits Confirm.
 */
export function TerminalThemeDialog({
  open,
  currentThemeId,
  onConfirm,
  onClose,
}: TerminalThemeDialogProps) {
  const { t } = useTranslation();
  const [selected, setSelected] = useState(currentThemeId);

  // Re-sync the selection each time the dialog opens.
  useEffect(() => {
    if (open) setSelected(currentThemeId);
  }, [open, currentThemeId]);

  const theme = getTerminalTheme(selected);

  return (
    <Dialog open={open} onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2 text-base">
            <Palette className="h-4 w-4 text-primary" />
            {t("termMenu.terminalTheme")}
          </DialogTitle>
          <DialogDescription>{t("termThemeDialog.desc")}</DialogDescription>
        </DialogHeader>

        <div className="grid grid-cols-[5rem_1fr] items-center gap-3">
          <span className="text-sm text-muted-foreground">{t("termThemeDialog.scheme")}</span>
          <Select value={selected} onChange={(e) => setSelected(e.target.value)}>
            {TERMINAL_THEMES.map((def) => (
              <option key={def.id} value={def.id}>
                {def.label}
              </option>
            ))}
          </Select>
        </div>

        {/* Live preview — a mock terminal rendered with the selected palette */}
        <div
          className="overflow-hidden rounded-[var(--radius)] border border-border font-mono text-[13px] leading-relaxed"
          style={{ background: theme.theme.background }}
        >
          <div className="border-b border-white/10 px-3 py-1 text-[11px]" style={{ color: theme.theme.brightBlack }}>
            {t("termThemeDialog.preview")}
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

        <DialogFooter>
          <Button variant="outline" onClick={onClose}>
            {t("common.cancel")}
          </Button>
          <Button
            className="bg-primary text-primary-foreground"
            onClick={() => onConfirm(selected)}
          >
            {t("common.confirm")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
