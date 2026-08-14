import { useMemo, useState } from "react";
import {
  Loader2,
  Plus,
  Server,
  Search,
  TerminalSquare,
  Pencil,
  Trash2,
  Folder,
  MousePointer2,
} from "lucide-react";
import { useTranslation } from "react-i18next";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import { useHosts, useDeleteHost, useOpenTerminal } from "@/features/hosts/hooks";
import { type HostDTO } from "@/features/hosts/api";
import { osInfo } from "@/features/hosts/osIcons";
import { useHostsUIStore } from "@/features/hosts/store";
import { useUIStore } from "@/stores/ui.store";
import { HostFormDialog } from "@/features/hosts/HostFormDialog";
import { useConfirm } from "@/lib/useConfirm";

/**
 * Host management view. Lists saved hosts grouped by their group
 * (test/stage/production/custom), with a search box that matches name, IP,
 * group, and tags. Each row has Connect / Edit / Delete actions.
 */
export function HostsView() {
  const { data: hosts, isLoading } = useHosts();
  const openEditor = useHostsUIStore((s) => s.openEditor);
  const editing = useHostsUIStore((s) => s.editing);
  const deleteHost = useDeleteHost();
  const openTerminal = useOpenTerminal();
  const setView = useUIStore((s) => s.setView);
  const [query, setQuery] = useState("");
  const { t } = useTranslation();
  const { askConfirm } = useConfirm();

  // Search: name, host IP, group, or tags.
  const filtered = useMemo(() => {
    if (!hosts) return [];
    const q = query.trim().toLowerCase();
    if (!q) return hosts;
    return hosts.filter((h) => {
      const haystack = [
        h.name,
        h.host,
        h.username,
        h.group,
        ...(h.tags ?? []),
      ]
        .join(" ")
        .toLowerCase();
      return haystack.includes(q);
    });
  }, [hosts, query]);

  // Group the filtered hosts by their group; ungrouped go into a "default" bucket.
  const grouped = useMemo(() => {
    const map = new Map<string, HostDTO[]>();
    for (const h of filtered) {
      const key = h.group || "";
      if (!map.has(key)) map.set(key, []);
      map.get(key)!.push(h);
    }
    // Deterministic order: production → stage → test → custom (alpha) → none.
    const order = ["production", "stage", "test"];
    const keys = [...map.keys()].sort((a, b) => {
      const ia = a === "" ? 99 : order.indexOf(a) >= 0 ? order.indexOf(a) : 50;
      const ib = b === "" ? 99 : order.indexOf(b) >= 0 ? order.indexOf(b) : 50;
      return ia - ib || a.localeCompare(b);
    });
    return keys.map((k) => ({ group: k, hosts: map.get(k)! }));
  }, [filtered]);

  const handleConnect = async (host: HostDTO) => {
    try {
      // useOpenTerminal's onSuccess already registers the session in the
      // terminal store — do NOT call addSession here or the session appears
      // twice (duplicate tabs).
      await openTerminal.mutateAsync({
        host: { id: host.id, name: host.name, terminalTheme: host.terminalTheme },
        creds: {},
      });
      setView("terminal");
    } catch {
      // open terminal failed (e.g. no credentials) — open editor instead
      openEditor(host);
    }
  };

  const handleDelete = async (host: HostDTO) => {
    const ok = await askConfirm({
      title: t("hosts.deleteTitle"),
      message: t("hosts.deleteConfirm", { name: host.name }),
      danger: true,
      confirmLabel: t("common.delete"),
    });
    if (!ok) return;
    await deleteHost.mutateAsync(host.id);
  };

  return (
    <div className="mx-auto max-w-4xl p-6">
      <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 className="text-xl font-semibold tracking-tight">{t("hosts.title")}</h1>
          <p className="mt-0.5 text-sm text-muted-foreground">
            {t("hosts.subtitle")}
          </p>
        </div>
        <Button onClick={() => openEditor("new")}>
          <Plus className="h-4 w-4" /> {t("hosts.addHost")}
        </Button>
      </div>

      {/* Search */}
      <div className="relative mb-4">
        <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
        <Input
          className="pl-9"
          placeholder={t("hosts.searchPlaceholder")}
          value={query}
          onChange={(e) => setQuery(e.target.value)}
        />
      </div>

      {isLoading ? (
        <div className="flex items-center justify-center py-16 text-sm text-muted-foreground">
          <Loader2 className="mr-2 h-4 w-4 animate-spin" /> Loading…
        </div>
      ) : !hosts || hosts.length === 0 ? (
        <EmptyState onAdd={() => openEditor("new")} />
      ) : filtered.length === 0 ? (
        <div className="py-16 text-center text-sm text-muted-foreground">
          {t("hosts.noMatch", { query })}
        </div>
      ) : (
        <div className="flex flex-col gap-6">
          {grouped.map(({ group, hosts: groupHosts }) => (
            <div key={group}>
              {/* Group header */}
              <div className="mb-2 flex items-center gap-2">
                <Folder className="h-4 w-4 text-primary" />
                <span className="text-sm font-semibold text-foreground">
                  {group === "" ? t("hosts.ungrouped") : group}
                </span>
                <Badge variant="outline" className="text-xs">
                  {groupHosts.length}
                </Badge>
              </div>

              <ul className="flex flex-col gap-2">
                {groupHosts.map((host) => (
                  <HostRow
                    key={host.id}
                    host={host}
                    onConnect={() => handleConnect(host)}
                    onEdit={() => openEditor(host)}
                    onDelete={() => handleDelete(host)}
                  />
                ))}
              </ul>
            </div>
          ))}
        </div>
      )}

      {/* Mount the dialog once; its visibility is driven by the store. */}
      {editing !== null && <HostFormDialog />}
    </div>
  );
}

function HostRow({
  host,
  onConnect,
  onEdit,
  onDelete,
}: {
  host: HostDTO;
  onConnect: () => void;
  onEdit: () => void;
  onDelete: () => void;
}) {
  const { t } = useTranslation();
  return (
    <li className="group flex items-center gap-3 rounded-[var(--radius)] border border-border bg-card p-3 transition-colors hover:border-primary/40 hover:bg-accent/40">
      <span className="flex h-9 w-9 shrink-0 items-center justify-center rounded-[var(--radius)] bg-secondary text-secondary-foreground">
        {osInfo(host.os) ? (
          // Distro icon once detected — makes hosts visually distinct.
          <img
            src={osInfo(host.os)!.icon}
            alt={osInfo(host.os)!.label}
            title={osInfo(host.os)!.label}
            className="os-icon h-4 w-4"
          />
        ) : (
          <Server className="h-4 w-4" />
        )}
      </span>

      {/* Main info — double-click to connect (single click is easily
          triggered by accident while scrolling/selecting). */}
      <div
        className="min-w-0 flex-1 cursor-default select-none text-left"
        onDoubleClick={onConnect}
        title="Double-click to open terminal"
      >
        <div className="flex items-center gap-2">
          <span className="truncate font-medium text-foreground">
            {host.name}
          </span>
          <OsBadge os={host.os} />
          <Badge variant="outline" className="shrink-0">
            {t(`hosts.authType.${host.authType}`)}
          </Badge>
          {host.group && (
            <Badge variant="secondary" className="shrink-0">
              {host.group}
            </Badge>
          )}
        </div>
        <div className="truncate font-mono text-xs text-muted-foreground">
          {host.username}@{host.host}:{host.port}
        </div>
        {host.tags && host.tags.length > 0 && (
          <div className="mt-1 flex flex-wrap gap-1">
            {host.tags.map((tag) => (
              <span
                key={tag}
                className="rounded-full bg-secondary/60 px-1.5 py-0.5 text-[10px] text-muted-foreground"
              >
                {tag}
              </span>
            ))}
          </div>
        )}
        {/* Subtle hint shown on hover so users discover the double-click. */}
        <span className="mt-0.5 inline-flex items-center gap-1 text-[10px] text-muted-foreground/70 opacity-0 transition-opacity group-hover:opacity-100">
          <MousePointer2 className="h-2.5 w-2.5" /> {t("hosts.doubleClickHint")}
        </span>
      </div>

      {/* Row actions — visible on hover */}
      <div className="flex shrink-0 items-center gap-0.5 opacity-0 transition-opacity group-hover:opacity-100">
        <button
          type="button"
          onClick={onConnect}
          title="Open terminal"
          className="rounded p-1.5 text-muted-foreground hover:bg-accent hover:text-primary"
        >
          <TerminalSquare className="h-4 w-4" />
        </button>
        <button
          type="button"
          onClick={onEdit}
          title="Edit"
          className="rounded p-1.5 text-muted-foreground hover:bg-accent hover:text-foreground"
        >
          <Pencil className="h-4 w-4" />
        </button>
        <button
          type="button"
          onClick={onDelete}
          title="Delete"
          className="rounded p-1.5 text-muted-foreground hover:bg-accent hover:text-destructive"
        >
          <Trash2 className="h-4 w-4" />
        </button>
      </div>
    </li>
  );
}

/** OS badge — read-only indicator with the distro icon + name. Hidden when the
 * host has no detected OS yet. */
function OsBadge({ os }: { os?: string | null }) {
  const info = osInfo(os);
  if (!info) return null;
  return (
    <span
      title={info.label}
      className="inline-flex shrink-0 items-center gap-1 rounded-full border border-border bg-muted/40 px-1.5 py-0.5 text-[10px] text-muted-foreground"
    >
      <img src={info.icon} alt="" className="os-icon h-3 w-3" />
      {info.label}
    </span>
  );
}

function EmptyState({ onAdd }: { onAdd: () => void }) {
  const { t } = useTranslation();
  return (
    <div className="flex flex-col items-center justify-center gap-3 rounded-[var(--radius)] border border-dashed border-border py-16 text-center">
      <Server className="h-10 w-10 text-muted-foreground" />
      <div>
        <h2 className="font-medium text-foreground">{t("hosts.noHostsTitle")}</h2>
        <p className="mt-1 max-w-sm text-sm text-muted-foreground">
          {t("hosts.noHostsDesc")}
        </p>
      </div>
      <Button onClick={onAdd} className="mt-1">
        <Plus className="h-4 w-4" /> {t("hosts.addFirst")}
      </Button>
    </div>
  );
}
