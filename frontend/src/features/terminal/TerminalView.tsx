import { TerminalSquare } from "lucide-react";

import { PlaceholderView } from "@/components/layout/PlaceholderView";

/**
 * SSH terminal view. Phase 1: placeholder.
 * Phase 2 will mount xterm.js wired to a Go PTY over an SSH shell session.
 */
export function TerminalView() {
  return (
    <PlaceholderView
      icon={TerminalSquare}
      title="SSH Terminal"
      description="A stable, PTY-backed terminal over SSH. Multi-tab sessions with resize, Ctrl+C, and long-lived reconnect."
      phase="Phase 2 — SSH Workspace"
    />
  );
}
