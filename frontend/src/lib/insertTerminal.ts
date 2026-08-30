import { TerminalService } from "@/../bindings/github.com/ai-remote/workspace/internal/interfaces";
import { useTerminalStore } from "@/features/terminal/terminal.store";
import { encodeBase64 } from "@/lib/base64";

/**
 * insertToTerminal types text into the ACTIVE terminal tab's input (as if
 * the user typed it) — NO trailing newline, so nothing executes until the
 * user presses Enter. This is the standard "hand off to the terminal"
 * channel shared by the agent's code blocks and the monitor/docker panels'
 * context menus (insert kill <pid>, docker exec …, ss -tnlp …). The user
 * keeps review authority; panels never execute anything themselves.
 *
 * Returns false when no SSH/local session is available to receive it.
 */
export function insertToTerminal(text: string): boolean {
  const { activeId, sessions } = useTerminalStore.getState();
  const sess = sessions.find((s) => s.id === activeId);
  if (!sess) return false;
  TerminalService.WriteStdin(sess.id, encodeBase64(text)).catch(() => {
    /* session may have closed */
  });
  return true;
}
