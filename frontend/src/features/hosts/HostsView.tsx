import { Loader2, Plus, Server } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { useHosts } from "@/features/hosts/hooks";
import { AUTH_TYPE_LABELS } from "@/features/hosts/api";
import { useHostsUIStore } from "@/features/hosts/store";
import { HostFormDialog } from "@/features/hosts/HostFormDialog";

/**
 * Host management view. Lists saved hosts with quick connect, and hosts the
 * add/edit/connect dialog.
 */
export function HostsView() {
  const { data: hosts, isLoading } = useHosts();
  const openEditor = useHostsUIStore((s) => s.openEditor);
  const editing = useHostsUIStore((s) => s.editing);

  return (
    <div className="mx-auto max-w-3xl p-6">
      <div className="mb-4 flex items-center justify-between">
        <div>
          <h1 className="text-xl font-semibold tracking-tight">Hosts</h1>
          <p className="mt-0.5 text-sm text-muted-foreground">
            Remote machines you can connect to.
          </p>
        </div>
        <Button onClick={() => openEditor("new")}>
          <Plus className="h-4 w-4" /> Add Host
        </Button>
      </div>

      {isLoading ? (
        <div className="flex items-center justify-center py-16 text-sm text-muted-foreground">
          <Loader2 className="mr-2 h-4 w-4 animate-spin" /> Loading…
        </div>
      ) : !hosts || hosts.length === 0 ? (
        <EmptyState onAdd={() => openEditor("new")} />
      ) : (
        <ul className="flex flex-col gap-2">
          {hosts.map((host) => (
            <li key={host.id}>
              <button
                type="button"
                onClick={() => openEditor(host)}
                className="flex w-full items-center gap-3 rounded-[var(--radius)] border border-border bg-card p-3 text-left transition-colors hover:border-primary/40 hover:bg-accent/40"
              >
                <span className="flex h-9 w-9 shrink-0 items-center justify-center rounded-[var(--radius)] bg-secondary text-secondary-foreground">
                  <Server className="h-4 w-4" />
                </span>
                <div className="min-w-0 flex-1">
                  <div className="flex items-center gap-2">
                    <span className="truncate font-medium text-foreground">
                      {host.name}
                    </span>
                    <Badge variant="outline" className="shrink-0">
                      {AUTH_TYPE_LABELS[host.authType] ?? host.authType}
                    </Badge>
                  </div>
                  <div className="truncate font-mono text-xs text-muted-foreground">
                    {host.username}@{host.host}:{host.port}
                  </div>
                </div>
              </button>
            </li>
          ))}
        </ul>
      )}

      {/* Mount the dialog once; its visibility is driven by the store. */}
      {editing !== null && <HostFormDialog />}
    </div>
  );
}

function EmptyState({ onAdd }: { onAdd: () => void }) {
  return (
    <div className="flex flex-col items-center justify-center gap-3 rounded-[var(--radius)] border border-dashed border-border py-16 text-center">
      <Server className="h-10 w-10 text-muted-foreground" />
      <div>
        <h2 className="font-medium text-foreground">No hosts yet</h2>
        <p className="mt-1 max-w-sm text-sm text-muted-foreground">
          Add a server to manage it — connect, open a terminal, and (in later
          phases) browse files.
        </p>
      </div>
      <Button onClick={onAdd} className="mt-1">
        <Plus className="h-4 w-4" /> Add your first host
      </Button>
    </div>
  );
}
