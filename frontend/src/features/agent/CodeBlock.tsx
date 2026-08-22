import { useState } from "react";
import { Check, Copy, CornerDownLeft, Terminal as TerminalIcon } from "lucide-react";
import { useTranslation } from "react-i18next";

import { cn } from "@/lib/utils";

interface CodeBlockProps {
  /** The code/script text. */
  code: string;
  /** Fence language from the markdown source (```bash …), when present. */
  language?: string;
  /** Whether a terminal session is available for "insert". */
  canInsert: boolean;
  /** Insert the code into the active terminal session (WriteStdin). */
  onInsert: (code: string) => void;
}

/**
 * Renders a fenced code block with Copy + Insert-to-Terminal actions.
 *
 * "Insert" sends the code to the active SSH session's stdin (via WriteStdin),
 * so the user can review an agent-suggested command and run it with one click
 * instead of copy-pasting manually.
 */
export function CodeBlock({ code, language, canInsert, onInsert }: CodeBlockProps) {
  const { t } = useTranslation();
  const [copied, setCopied] = useState(false);
  const [inserted, setInserted] = useState(false);

  const trimmed = code.replace(/^\n+|\n+$/g, "");

  const handleCopy = async () => {
    try {
      await navigator.clipboard.writeText(trimmed);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch {
      // clipboard may be blocked; ignore
    }
  };

  const handleInsert = () => {
    onInsert(trimmed);
    setInserted(true);
    setTimeout(() => setInserted(false), 2000);
  };

  return (
    <div className="group relative my-1 overflow-hidden rounded-[var(--radius)] border border-border bg-background/80">
      {/* Action bar */}
      <div className="flex items-center justify-between border-b border-border/60 bg-secondary/40 px-2 py-1">
        <span className="flex items-center gap-1 text-[10px] uppercase tracking-wide text-muted-foreground">
          <TerminalIcon className="h-3 w-3" />
          {language || detectKind(trimmed, t)}
        </span>
        <div className="flex items-center gap-0.5">
          <button
            type="button"
            onClick={handleCopy}
            className="flex items-center gap-1 rounded px-1.5 py-0.5 text-[11px] text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
            title={t("agent.copy")}
          >
            {copied ? (
              <Check className="h-3 w-3 text-success" />
            ) : (
              <Copy className="h-3 w-3" />
            )}
            {copied ? t("agent.copied") : t("agent.copy")}
          </button>
          {canInsert && (
            <button
              type="button"
              onClick={handleInsert}
              className={cn(
                "flex items-center gap-1 rounded px-1.5 py-0.5 text-[11px] transition-colors",
                inserted
                  ? "text-success"
                  : "text-primary hover:bg-accent",
              )}
              title={t("agent.insertHint")}
            >
              {inserted ? (
                <Check className="h-3 w-3" />
              ) : (
                <CornerDownLeft className="h-3 w-3" />
              )}
              {inserted ? t("agent.inserted") : t("agent.insert")}
            </button>
          )}
        </div>
      </div>
      {/* Code body */}
      <pre className="overflow-auto whitespace-pre-wrap break-all p-2.5 font-mono text-xs leading-relaxed text-foreground/90">
        {trimmed}
      </pre>
    </div>
  );
}

// detectKind guesses the language/label from the content.
function detectKind(code: string, t: (k: string) => string): string {
  const firstLine = code.split("\n")[0]?.trim() ?? "";
  if (firstLine.startsWith("#!")) return t("agent.codeScript");
  if (/^(apt|yum|dnf|systemctl|docker|kubectl|git|ssh|curl|wget)\s/.test(firstLine)) {
    return t("agent.codeShell");
  }
  // single short line starting with a common command → "command"
  if (!code.includes("\n") && firstLine.length < 120) return t("agent.codeCommand");
  return t("agent.codeScript");
}
