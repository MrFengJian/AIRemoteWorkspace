import { useMemo, useState } from "react";
import {
  Search,
  Plus,
  Server,
  TerminalSquare,
  Pencil,
  Trash2,
  Folder,
  ChevronRight,
  Monitor,
} from "lucide-react";
import { useTranslation } from "react-i18next";

import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { useHosts, useOpenTerminal, useOpenLocalTerminal, useDeleteHost } from "@/features/hosts/hooks";
import { type HostDTO } from "@/features/hosts/api";
import { osInfo } from "@/features/hosts/osIcons";
import { useHostsUIStore } from "@/features/hosts/store";
import { useTerminalStore } from "@/features/terminal/terminal.store";
import { TerminalTabMenu, type MenuItem } from "@/features/terminal/TerminalTabMenu";
import { useConfirm } from "@/lib/useConfirm";
import { cn } from "@/lib/utils";

/**
 * HostsSidebar — the Xshell-style session manager living inside the terminal
 * workspace: searchable, grouped host list with live connection state.
 * Single-click activates the host's open tab (or connects); the context menu
 * forces a new tab / edits / deletes. Host CRUD lives entirely here — there
 * is no standalone hosts page anymore.
 */
export function HostsSidebar() {
  const { t } = useTranslation();
  const { data: hosts, isLoading } = useHosts();
  const [query, setQuery] = useState("");
  const [collapsed, setCollapsed] = useState<Record<string, boolean>>({});
  const [menu, setMenu] = useState<{ host: HostDTO; x: number; y: number } | null>(null);

  const openEditor = useHostsUIStore((s) => s.openEditor);
  const openTerminal = useOpenTerminal();
  const openLocal = useOpenLocalTerminal();
  const deleteHost = useDeleteHost();
  const sessions = useTerminalStore((s) => s.sessions);
  const setActive = useTerminalStore((s) => s.setActive);
  const { askConfirm } = useConfirm();

  // Search: name, host IP, group, or tags.
  const filtered = useMemo(() => {
    if (!hosts) return [];
    const q = query.trim().toLowerCase();
    if (!q) return hosts;
    return hosts.filter((h) =>
      [h.name, h.host, h.username, h.group, ...(h.tags ?? [])]
        .join(" ")
        .toLowerCase()
        .includes(q),
    );
  }, [hosts, query]);

  // Group the filtered hosts; deterministic order production → stage → test
  // → custom (alpha) → none.
  const grouped = useMemo(() => {
    const map = new Map<string, HostDTO[]>();
    for (const h of filtered) {
      const key = h.group || "";
      if (!map.has(key)) map.set(key, []);
      map.get(key)!.push(h);
    }
    const order = ["production", "stage", "test"];
    const keys = [...map.keys()].sort((a, b) => {
      const ia = a === "" ? 99 : order.indexOf(a) >= 0 ? order.indexOf(a) : 50;
      const ib = b === "" ? 99 : order.indexOf(b) >= 0 ? order.indexOf(b) : 50;
      return ia - ib || a.localeCompare(b);
    });
    return keys.map((k) => ({ group: k, hosts: map.get(k)! }));
  }, [filtered]);

  /** Latest live (connected/connecting) session for a host, if any. */
  const liveSession = (hostID: string) => {
    const live = sessions.filter(
      (s) => s.hostID === hostID && (s.status === "connected" || s.status === "connecting"),
    );
    return live.length > 0 ? live[live.length - 1] : null;
  };

  /** Connection-state dot colour for a host (latest live session wins). */
  const hostStatus = (hostID: string): "connected" | "connecting" | null => {
    const s = liveSession(hostID);
    if (!s) return null;
    // liveSession only yields connected/connecting sessions.
    return s.status as "connected" | "connecting";
  };

  /** Click: activate the host's live tab, or connect a new one. */
  const activateOrConnect = (host: HostDTO) => {
    const live = liveSession(host.id);
    if (live) {
      setActive(live.id);
      return;
    }
    openTerminal
      .mutateAsync({
        host: {
          id: host.id,
          name: host.name,
          terminalTheme: host.terminalTheme,
          terminalFont: host.terminalFont,
          terminalFontSize: host.terminalFontSize,
        },
        creds: {},
      })
      .catch(() => {
        // Connection failed (e.g. no remembered credentials) — the reason is
        // toasted globally; open the editor to fix credentials right away.
        openEditor(host);
      });
  };

  const forceConnect = (host: HostDTO) => {
    openTerminal
      .mutateAsync({
        host: {
          id: host.id,
          name: host.name,
          terminalTheme: host.terminalTheme,
          terminalFont: host.terminalFont,
          terminalFontSize: host.terminalFontSize,
        },
        creds: {},
      })
      .catch(() => {
        openEditor(host);
      });
  };

  const handleDelete = async (host: HostDTO) => {
    const ok = await askConfirm({
      title: t("hosts.deleteTitle"),
      message: t("hosts.deleteConfirm", { name: host.name }),
      danger: true,
      confirmLabel: t("common.delete"),
    });
    if (!ok) return;
    deleteHost.mutateAsync(host.id).catch(() => {});
  };

  const buildMenuItems = (host: HostDTO): MenuItem[] => [
    {
      label: t("hosts.newTab"),
      icon: TerminalSquare,
      onClick: () => forceConnect(host),
    },
    {
      label: t("common.edit"),
      icon: Pencil,
      onClick: () => openEditor(host),
    },
    { type: "separator" },
    {
      label: t("common.delete"),
      icon: Trash2,
      danger: true,
      onClick: () => handleDelete(host),
    },
  ];

  return (
    <aside
      className="flex h-full w-60 shrink-0 flex-col border-r border-border bg-card"
      aria-label={t("nav.hosts")}
    >
      {/* Search + add */}
      <div className="flex items-center gap-1.5 p-2">
        <div className="relative min-w-0 flex-1">
          <Search className="pointer-events-none absolute left-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground" />
          <Input
            className="h-8 pl-8 text-xs"
            placeholder={t("hosts.searchPlaceholder")}
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            aria-label={t("hosts.searchPlaceholder")}
          />
        </div>
        <Button
          variant="ghost"
          size="icon"
          className="h-8 w-8 shrink-0"
          onClick={() => openEditor("new")}
          aria-label={t("hosts.addHost")}
          title={t("hosts.addHost")}
        >
          <Plus className="h-4 w-4" />
        </Button>
      </div>

      {/* Host list */}
      <div className="min-h-0 flex-1 overflow-auto px-1.5 pb-2">
        {isLoading ? (
          <div className="flex items-center justify-center py-8 text-muted-foreground">
            <Search className="h-4 w-4 animate-pulse" />
          </div>
        ) : !hosts || hosts.length === 0 ? (
          <div className="flex flex-col items-center gap-2 px-3 py-8 text-center">
            <Server className="h-6 w-6 text-muted-foreground" />
            <p className="text-xs text-muted-foreground">{t("hosts.sidebarEmpty")}</p>
            <Button variant="outline" size="sm" onClick={() => openEditor("new")}>
              <Plus className="h-3.5 w-3.5" /> {t("hosts.addHost")}
            </Button>
          </div>
        ) : filtered.length === 0 ? (
          <p className="px-3 py-8 text-center text-xs text-muted-foreground">
            {t("hosts.noMatch", { query })}
          </p>
        ) : (
          grouped.map(({ group, hosts: groupHosts }) => {
            const isCollapsed = collapsed[group] === true;
            return (
              <div key={group} className="mb-0.5">
                <button
                  type="button"
                  onClick={() => setCollapsed((c) => ({ ...c, [group]: !isCollapsed }))}
                  aria-expanded={!isCollapsed}
                  className="flex w-full items-center gap-1.5 rounded-[var(--radius)] px-2 py-1 text-[11px] font-medium text-muted-foreground transition-colors hover:bg-accent/40 hover:text-foreground"
                >
                  <ChevronRight
                    className={cn("h-3 w-3 transition-transform", !isCollapsed && "rotate-90")}
                  />
                  <Folder className="h-3 w-3 text-primary" />
                  <span className="truncate">
                    {group === "" ? t("hosts.ungrouped") : group}
                  </span>
                  <span className="ml-auto text-[10px] opacity-60">{groupHosts.length}</span>
                </button>
                {!isCollapsed && (
                  <ul className="mt-0.5 flex flex-col gap-0.5">
                    {groupHosts.map((host) => (
                      <HostRow
                        key={host.id}
                        host={host}
                        status={hostStatus(host.id)}
                        onActivate={() => activateOrConnect(host)}
                        onForceConnect={() => forceConnect(host)}
                        onEdit={() => openEditor(host)}
                        onDelete={() => handleDelete(host)}
                        onContextMenu={(e) => {
                          e.preventDefault();
                          setMenu({ host, x: e.clientX, y: e.clientY });
                        }}
                      />
                    ))}
                  </ul>
                )}
              </div>
            );
          })
        )}
      </div>

      {/* Footer: local terminal entry */}
      <div className="border-t border-border p-1.5">
        <button
          type="button"
          disabled={openLocal.isPending}
          onClick={() => openLocal.mutate(t("terminal.localTab"))}
          className="flex w-full items-center gap-1.5 rounded-[var(--radius)] px-2 py-1.5 text-xs text-muted-foreground transition-colors hover:bg-accent/50 hover:text-foreground disabled:opacity-50"
        >
          <Monitor className="h-3.5 w-3.5" />
          {t("terminal.newLocalTab")}
        </button>
      </div>

      {/* Context menu */}
      {menu && (
        <TerminalTabMenu
          x={menu.x}
          y={menu.y}
          onClose={() => setMenu(null)}
          items={buildMenuItems(menu.host)}
        />
      )}
    </aside>
  );
}

function HostRow({
  host,
  status,
  onActivate,
  onForceConnect,
  onEdit,
  onDelete,
  onContextMenu,
}: {
  host: HostDTO;
  status: "connected" | "connecting" | null;
  onActivate: () => void;
  onForceConnect: () => void;
  onEdit: () => void;
  onDelete: () => void;
  onContextMenu: (e: React.MouseEvent) => void;
}) {
  const { t } = useTranslation();
  return (
    <li
      className="group flex cursor-pointer items-center gap-2 rounded-[var(--radius)] px-2 py-1.5 transition-colors hover:bg-accent/40"
      onClick={onActivate}
      onContextMenu={onContextMenu}
      title={host.name}
    >
      <span className="flex h-6 w-6 shrink-0 items-center justify-center rounded-[calc(var(--radius)-2px)] bg-secondary text-secondary-foreground">
        {osInfo(host.os) ? (
          <img
            src={osInfo(host.os)!.icon}
            alt={osInfo(host.os)!.label}
            className="os-icon h-3.5 w-3.5"
          />
        ) : (
          <Server className="h-3.5 w-3.5" />
        )}
      </span>
      <span className="min-w-0 flex-1">
        <span className="block truncate text-xs font-medium text-foreground">{host.name}</span>
        <span className="block truncate font-mono text-[10px] text-muted-foreground">
          {host.username}@{host.host}
        </span>
      </span>
      {/* Connection state dot */}
      {status && (
        <span
          className={cn(
            "h-1.5 w-1.5 shrink-0 rounded-full",
            status === "connected" && "bg-success",
            status === "connecting" && "bg-warning animate-pulse",
          )}
          aria-label={status}
        />
      )}
      {/* Row actions — visible on hover and when keyboard-focused within */}
      <span className="flex shrink-0 items-center gap-0.5 opacity-0 transition-opacity focus-within:opacity-100 group-hover:opacity-100">
        <button
          type="button"
          onClick={(e) => {
            e.stopPropagation();
            onForceConnect();
          }}
          title={t("hosts.newTab")}
          aria-label={t("hosts.newTab")}
          className="rounded p-1 text-muted-foreground hover:bg-accent hover:text-primary focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        >
          <TerminalSquare className="h-3.5 w-3.5" />
        </button>
        <button
          type="button"
          onClick={(e) => {
            e.stopPropagation();
            onEdit();
          }}
          title={t("common.edit")}
          aria-label={t("common.edit")}
          className="rounded p-1 text-muted-foreground hover:bg-accent hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        >
          <Pencil className="h-3.5 w-3.5" />
        </button>
        <button
          type="button"
          onClick={(e) => {
            e.stopPropagation();
            onDelete();
          }}
          title={t("common.delete")}
          aria-label={t("common.delete")}
          className="rounded p-1 text-muted-foreground hover:bg-accent hover:text-destructive focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        >
          <Trash2 className="h-3.5 w-3.5" />
        </button>
      </span>
    </li>
  );
}
