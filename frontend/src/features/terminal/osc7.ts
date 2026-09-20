// OSC 7 shell-integration activation for the SFTP follow-session-cwd
// feature — with client-side echo suppression.
//
// Why injection at all: the terminal cannot know a remote shell's cwd unless
// the shell reports it. The standard channel is OSC 7, but bash/zsh only emit
// it when configured (PROMPT_COMMAND / precmd). For sessions whose shell does
// not report natively, we type a one-line integration into the interactive
// shell once. The line WOULD normally echo — that is the visibility problem —
// so sending it arms an echo-suppression window in the terminal pane
// (TerminalPanel consults consumeSuppression before rendering): the echoed
// command, the following prompt, everything until the OSC 7 response arrives
// is dropped. The user sees nothing. Caps (bytes + deadline) bound the window
// so a busy session can never lose more than a bounded slice of output.
//
// Tabby-style alternative (injecting at session STARTUP by wrapping the shell
// command) is echo-free by construction but requires rewriting session
// startup for every shell family — deliberately not done here.
import { TerminalService } from "@/../bindings/github.com/ai-remote/workspace/internal/interfaces";
import { encodeBase64 } from "@/lib/base64";

/**
 * POSIX-sh/bash/zsh polyglot: installs an `_zz_osc7` prompt hook that emits
 * the working directory via OSC 7 on every prompt redraw (zsh → precmd hook,
 * sh/bash → PROMPT_COMMAND), guarded against double-install.
 */
const OSC7_INTEGRATION =
  "eval '_zz_osc7(){ printf \"\\033]7;file://%s%s\\007\" \"${HOSTNAME:-${HOST:-localhost}}\" \"$PWD\"; }; " +
  "if [ -n \"$ZSH_VERSION\" ]; then autoload -Uz add-zsh-hook; add-zsh-hook precmd _zz_osc7; " +
  "else case \";${PROMPT_COMMAND:-};\" in *\";_zz_osc7;\"*) ;; *) PROMPT_COMMAND=\"_zz_osc7;${PROMPT_COMMAND}\";; esac; fi; _zz_osc7'\n";

/** Sessions whose shell already received the integration (webview lifetime). */
const injected = new Set<string>();

/** Active echo-suppression windows, keyed by session id. */
const suppression = new Map<string, { remaining: number; deadline: number }>();

/** Caps bounding the suppression window (bytes echoed / wall-clock ms). */
const SUPPRESS_MAX_BYTES = 2048;
const SUPPRESS_MAX_MS = 4000;

export function isShellIntegrationInjected(sessionID: string): boolean {
  return injected.has(sessionID);
}

/**
 * Send the integration line once and arm the echo-suppression window. The
 * pane's output handler drops rendered bytes via consumeSuppression until the
 * OSC 7 response arrives (disarmShellIntegration) or a cap expires.
 */
export function injectShellIntegration(sessionID: string): void {
  injected.add(sessionID);
  suppression.set(sessionID, {
    remaining: SUPPRESS_MAX_BYTES,
    deadline: Date.now() + SUPPRESS_MAX_MS,
  });
  TerminalService.WriteStdin(sessionID, encodeBase64(OSC7_INTEGRATION)).catch(() => {
    injected.delete(sessionID);
    suppression.delete(sessionID);
  });
}

/**
 * Consume `byteCount` rendered bytes while the suppression window is active.
 * Returns true when the caller must DROP these bytes (they are the echoed
 * command), false once the window is disarmed (normal rendering resumes).
 * Callers MUST first run the OSC 7 sniffer: receiving the integration's own
 * OSC 7 response disarms the window immediately.
 */
export function consumeSuppression(sessionID: string, byteCount: number): boolean {
  const win = suppression.get(sessionID);
  if (!win) return false;
  if (Date.now() > win.deadline || win.remaining <= 0) {
    suppression.delete(sessionID);
    return false;
  }
  win.remaining -= byteCount;
  if (win.remaining <= 0) {
    suppression.delete(sessionID);
    return false;
  }
  return true;
}

/** Disarm the suppression window (called when the OSC 7 response arrives —
 *  from this point on every byte is real output and must render). */
export function disarmShellIntegration(sessionID: string): void {
  suppression.delete(sessionID);
}
