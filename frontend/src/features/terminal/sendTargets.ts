import { useEffect, useMemo, useState } from "react";

import { TerminalService } from "@/../bindings/github.com/ai-remote/workspace/internal/interfaces";
import { useTerminalStore, type TerminalSession } from "@/features/terminal/terminal.store";
import { encodeBase64 } from "@/lib/base64";

/**
 * Shared target-selection primitive for the bars that batch-send input to
 * open sessions (快速命令栏 / 撰写栏 — two independent features that behave
 * identically here): tracks which live tabs are targeted, follows the active
 * tab until the user picks targets by hand, prunes closed tabs, and exposes
 * the payload writer used by every send path.
 */
export function useSendTargets() {
  const sessions = useTerminalStore((s) => s.sessions);
  const activeId = useTerminalStore((s) => s.activeId);

  // Target selection = tab ids. Follows the active tab until the user picks
  // targets by hand (dirty), after which the explicit selection stands.
  const [targets, setTargets] = useState<Set<string>>(() => new Set());
  const [dirty, setDirty] = useState(false);

  // Only live tabs can receive input; closed/error tabs would silently drop it.
  const sendable = useMemo(
    () => sessions.filter((s) => s.status === "connected"),
    [sessions],
  );

  // Follow the active tab while the user hasn't customized the selection.
  useEffect(() => {
    if (dirty) return;
    setTargets(
      activeId && sendable.some((s) => s.id === activeId)
        ? new Set([activeId])
        : new Set(),
    );
  }, [activeId, dirty, sendable]);

  // Prune closed tabs out of an explicit selection so stale ids never linger.
  useEffect(() => {
    setTargets((prev) => {
      const live = new Set(sessions.map((s) => s.id));
      const next = new Set([...prev].filter((id) => live.has(id)));
      return next.size === prev.size ? prev : next;
    });
  }, [sessions]);

  const toggleTarget = (id: string) => {
    setDirty(true);
    setTargets((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  };

  const selectAllTargets = () => {
    setDirty(true);
    setTargets(new Set(sendable.map((s) => s.id)));
  };

  const selectNoTargets = () => {
    setDirty(true);
    setTargets(new Set());
  };

  const targetTabs = sendable.filter((s) => targets.has(s.id));

  return { sessions, sendable, targets, targetTabs, toggleTarget, selectAllTargets, selectNoTargets };
}

/**
 * sendPayloadToTabs writes the exact PTY payload to every pane of the given
 * tabs (split panes are shells on the same target) and returns how many tabs
 * received it. Text is written raw — line breaks should already be carriage
 * returns, so multi-line payloads execute line by line, exactly as if typed.
 */
export function sendPayloadToTabs(
  sessions: TerminalSession[],
  payload: string,
  tabIds: string[],
): number {
  if (payload === "" || tabIds.length === 0) return 0;
  let sent = 0;
  for (const tabId of tabIds) {
    const tab = sessions.find((s) => s.id === tabId);
    if (!tab) continue;
    for (const paneId of tab.paneIds) {
      TerminalService.WriteStdin(paneId, encodeBase64(payload)).catch(() => {
        /* session may have just closed */
      });
    }
    sent++;
  }
  return sent;
}
