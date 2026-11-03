import { useState } from "react";
import { Pin, Server } from "lucide-react";
import { useTranslation } from "react-i18next";

import { useHosts } from "@/features/hosts/hooks";
import { osInfo } from "@/features/hosts/osIcons";
import { useQuickLinksStore } from "@/features/hosts/quicklinks.store";
import { useActivateOrConnect } from "@/features/hosts/useActivateOrConnect";
import type { HostDTO } from "@/features/hosts/api";
import { useTerminalStore } from "@/features/terminal/terminal.store";
import { useUIStore } from "@/stores/ui.store";
import { TerminalTabMenu, type MenuItem } from "@/features/terminal/TerminalTabMenu";
import { cn } from "@/lib/utils";

/**
 * Bottom bar: the user's quick links — pinned hosts as one-click connect
 * chips. Replaces the old status bar (app version / platform / clock): those
 * were ambient chrome, the quick links are something you actually click.
 * `onOpen` decides what activating a host means — the main window jumps to
 * the host's terminal tab, the standalone SFTP window opens that host's
 * file window.
 */
export function QuickLinksBar({ onOpen }: { onOpen?: (host: HostDTO) => void }) {
  const { t } = useTranslation();
  const { data: hosts } = useHosts();
  const ids = useQuickLinksStore((s) => s.ids);
  const toggle = useQuickLinksStore((s) => s.toggle);
  const { activateOrConnect } = useActivateOrConnect();
  const sessions = useTerminalStore((s) => s.sessions);
  const [menu, setMenu] = useState<{ host: HostDTO; x: number; y: number } | null>(null);

  /** Pinned ids resolved against the live host list (stale ids vanish). */
  const linked = ids
    .map((id) => (hosts ?? []).find((h) => h.id === id))
    .filter((h): h is HostDTO => Boolean(h));

  const openHost = (host: HostDTO) => {
    if (onOpen) {
      onOpen(host);
      return;
    }
    useUIStore.getState().setView("terminal");
    activateOrConnect(host);
  };

  /** Live (connected/connecting) session for the host — drives the dot. */
  const isLive = (hostID: string) =>
    sessions.some((s) => s.hostID === hostID && (s.status === "connected" || s.status === "connecting"));

  const menuItems = (host: HostDTO): MenuItem[] => [
    {
      label: t("hosts.openTerminal"),
      icon: Server,
      onClick: () => openHost(host),
    },
    {
      label: t("hosts.quickLinkRemove"),
      icon: Pin,
      onClick: () => toggle(host.id),
    },
  ];

  return (
    <footer className="flex h-6 items-center gap-1.5 border-t border-border bg-card px-2 text-xs text-muted-foreground">
      {linked.length === 0 ? (
        <span className="truncate px-1 text-[10px] text-muted-foreground/70">
          {t("hosts.quickLinkEmpty")}
        </span>
      ) : (
        <>
          <span className="shrink-0 px-0.5 text-[10px] text-muted-foreground/60">
            {t("hosts.quickLinkLabel")}
          </span>
          <div className="flex min-w-0 flex-1 items-center gap-1 overflow-x-auto">
            {linked.map((host) => (
              <button
                key={host.id}
                type="button"
                onClick={() => openHost(host)}
                onContextMenu={(e) => {
                  e.preventDefault();
                  setMenu({ host, x: e.clientX, y: e.clientY });
                }}
                title={`${host.name} · ${host.username}@${host.host}`}
                className="flex h-5 shrink-0 items-center gap-1 rounded-[calc(var(--radius)-2px)] px-1.5 text-[10px] text-muted-foreground transition-colors hover:bg-accent/60 hover:text-foreground"
              >
                {osInfo(host.os) ? (
                  <img src={osInfo(host.os)!.icon} alt="" className="os-icon h-2.5 w-2.5" />
                ) : (
                  <Server className="h-2.5 w-2.5" />
                )}
                <span className="max-w-[10rem] truncate">{host.name}</span>
                <span
                  className={cn(
                    "h-1 w-1 shrink-0 rounded-full",
                    isLive(host.id) ? "bg-success" : "bg-muted-foreground/40",
                  )}
                />
              </button>
            ))}
          </div>
        </>
      )}
      {menu && (
        <TerminalTabMenu
          x={menu.x}
          y={menu.y}
          onClose={() => setMenu(null)}
          items={menuItems(menu.host)}
        />
      )}
    </footer>
  );
}
