import { useEffect, useState } from "react";
import { Loader2, Plug, Save, Trash2 } from "lucide-react";

import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select } from "@/components/ui/select";
import { Badge } from "@/components/ui/badge";
import {
  AUTH_TYPES,
  AUTH_TYPE_LABELS,
  type AuthType,
  type CredentialsDTO,
  type HostDTO,
  type HostInputDTO,
} from "@/features/hosts/api";
import {
  useCreateHost,
  useDeleteHost,
  useTestConnection,
  useUpdateHost,
  useOpenTerminal,
} from "@/features/hosts/hooks";
import { useHostsUIStore } from "@/features/hosts/store";
import { useUIStore } from "@/stores/ui.store";

const EMPTY_INPUT: HostInputDTO = {
  name: "",
  host: "",
  port: 22,
  username: "",
  authType: "key",
  keyPath: "",
};

const EMPTY_CREDS: CredentialsDTO = { password: "", keyPath: "", keyPassphrase: "", useAgent: false };

/**
 * HostFormDialog — create/edit/delete + test-connect + open-terminal.
 *
 * Credentials (password / passphrase) are session-only: they live in local
 * state, are sent over the binding at action time, and are never persisted.
 * Phase 5's SecretStore will replace the manual entry with OS-keychain lookups.
 */
export function HostFormDialog() {
  const editing = useHostsUIStore((s) => s.editing);
  const closeEditor = useHostsUIStore((s) => s.closeEditor);
  const setView = useUIStore((s) => s.setView);

  const createHost = useCreateHost();
  const updateHost = useUpdateHost();
  const deleteHost = useDeleteHost();
  const testConnection = useTestConnection();
  const openTerminal = useOpenTerminal();

  const isOpen = editing !== null;
  const existing: HostDTO | null = editing && editing !== "new" ? editing : null;

  const [input, setInput] = useState<HostInputDTO>(EMPTY_INPUT);
  const [creds, setCreds] = useState<CredentialsDTO>(EMPTY_CREDS);
  const [testResult, setTestResult] = useState<string | null>(null);

  // Sync local form state when the dialog target changes.
  useEffect(() => {
    if (!isOpen) return;
    if (existing) {
      setInput({
        name: existing.name,
        host: existing.host,
        port: existing.port,
        username: existing.username,
        authType: (existing.authType || "key") as AuthType,
        keyPath: "",
      });
    } else {
      setInput(EMPTY_INPUT);
    }
    setCreds(EMPTY_CREDS);
    setTestResult(null);
  }, [isOpen, existing]);

  const update = (patch: Partial<HostInputDTO>) =>
    setInput((v) => ({ ...v, ...patch }));
  const updateCreds = (patch: Partial<CredentialsDTO>) =>
    setCreds((v) => ({ ...v, ...patch }));

  const buildCreds = (): CredentialsDTO => ({
    password: input.authType === "password" ? creds.password : "",
    keyPath: input.authType === "key" ? (creds.keyPath || input.keyPath) : "",
    keyPassphrase: input.authType === "key" ? creds.keyPassphrase : "",
    useAgent: input.authType === "agent",
  });

  const handleSave = async () => {
    try {
      if (existing) {
        await updateHost.mutateAsync({ id: existing.id, input });
      } else {
        await createHost.mutateAsync(input);
      }
      closeEditor();
    } catch {
      /* mutation error surfaces via the hook's state; keep dialog open */
    }
  };

  const handleDelete = async () => {
    if (!existing) return;
    if (!confirm(`Delete host "${existing.name}"? This cannot be undone.`)) return;
    await deleteHost.mutateAsync(existing.id);
    closeEditor();
  };

  const handleTest = async () => {
    if (!existing) {
      // For an unsaved host, persist first so we have an id to test against.
      const created = await createHost.mutateAsync(input);
      await runTest(created.id);
    } else {
      await runTest(existing.id);
    }
  };

  const runTest = async (hostId: string) => {
    setTestResult(null);
    try {
      const res = await testConnection.mutateAsync({
        hostId,
        creds: buildCreds(),
      });
      setTestResult(res.ok ? "✓ Connected successfully" : `✗ ${res.msg}`);
    } catch (e) {
      setTestResult(`✗ ${e instanceof Error ? e.message : String(e)}`);
    }
  };

  const handleConnect = async () => {
    if (!existing) return;
    try {
      await openTerminal.mutateAsync({ host: { id: existing.id, name: existing.name }, creds: buildCreds() });
      closeEditor();
      setView("terminal");
    } catch {
      /* keep dialog open so the error is visible */
    }
  };

  const busy =
    createHost.isPending ||
    updateHost.isPending ||
    deleteHost.isPending ||
    testConnection.isPending ||
    openTerminal.isPending;

  return (
    <Dialog
      open={isOpen}
      onOpenChange={(o) => {
        if (!o) closeEditor();
      }}
    >
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{existing ? "Edit Host" : "Add Host"}</DialogTitle>
          <DialogDescription>
            Configure a remote machine. Credentials are used only for this
            session — they are never saved to disk (Phase 5 will add keychain
            storage).
          </DialogDescription>
        </DialogHeader>

        <form
          className="grid gap-4"
          onSubmit={(e) => {
            e.preventDefault();
            handleSave();
          }}
        >
          <div className="grid grid-cols-2 gap-3">
            <div className="col-span-2 grid gap-1.5">
              <Label htmlFor="name">Name</Label>
              <Input
                id="name"
                value={input.name}
                onChange={(e) => update({ name: e.target.value })}
                placeholder="production-web-1"
                required
              />
            </div>

            <div className="grid gap-1.5">
              <Label htmlFor="host">Host</Label>
              <Input
                id="host"
                value={input.host}
                onChange={(e) => update({ host: e.target.value })}
                placeholder="10.0.0.1 or example.com"
                required
              />
            </div>
            <div className="grid gap-1.5">
              <Label htmlFor="port">Port</Label>
              <Input
                id="port"
                type="number"
                min={1}
                max={65535}
                value={input.port}
                onChange={(e) => update({ port: Number(e.target.value) || 22 })}
              />
            </div>

            <div className="grid gap-1.5">
              <Label htmlFor="username">Username</Label>
              <Input
                id="username"
                value={input.username}
                onChange={(e) => update({ username: e.target.value })}
                placeholder="root"
                required
              />
            </div>
            <div className="grid gap-1.5">
              <Label htmlFor="authType">Authentication</Label>
              <Select
                id="authType"
                value={input.authType}
                onChange={(e) => update({ authType: e.target.value as AuthType })}
              >
                {AUTH_TYPES.map((t) => (
                  <option key={t} value={t}>
                    {AUTH_TYPE_LABELS[t]}
                  </option>
                ))}
              </Select>
            </div>

            {input.authType === "key" && (
              <div className="col-span-2 grid gap-1.5">
                <Label htmlFor="keyPath">Private key path (saved)</Label>
                <Input
                  id="keyPath"
                  value={input.keyPath}
                  onChange={(e) => update({ keyPath: e.target.value })}
                  placeholder="C:\Users\you\.ssh\id_ed25519"
                />
              </div>
            )}
          </div>

          {/* Credentials — session only, never persisted. */}
          <fieldset className="grid gap-3 rounded-[var(--radius)] border border-border bg-background/50 p-3">
            <legend className="px-1.5 text-xs text-muted-foreground">
              Credentials (this session only)
            </legend>
            {input.authType === "password" && (
              <div className="grid gap-1.5">
                <Label htmlFor="password">Password</Label>
                <Input
                  id="password"
                  type="password"
                  value={creds.password}
                  onChange={(e) => updateCreds({ password: e.target.value })}
                  placeholder="Enter password"
                />
              </div>
            )}
            {input.authType === "key" && (
              <>
                <div className="grid gap-1.5">
                  <Label htmlFor="credsKeyPath">Key file (override)</Label>
                  <Input
                    id="credsKeyPath"
                    value={creds.keyPath}
                    onChange={(e) => updateCreds({ keyPath: e.target.value })}
                    placeholder={input.keyPath || "path to private key"}
                  />
                </div>
                <div className="grid gap-1.5">
                  <Label htmlFor="passphrase">Passphrase (if encrypted)</Label>
                  <Input
                    id="passphrase"
                    type="password"
                    value={creds.keyPassphrase}
                    onChange={(e) => updateCreds({ keyPassphrase: e.target.value })}
                  />
                </div>
              </>
            )}
            {input.authType === "agent" && (
              <p className="text-xs text-muted-foreground">
                Uses ssh-agent via SSH_AUTH_SOCK. No credential entry needed.
              </p>
            )}
          </fieldset>

          {testResult && (
            <div className="flex items-center gap-2">
              <Badge variant={testResult.startsWith("✓") ? "success" : "destructive"}>
                {testResult}
              </Badge>
            </div>
          )}
        </form>

        <DialogFooter className="flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
          <div>
            {existing && (
              <Button
                type="button"
                variant="ghost"
                size="sm"
                onClick={handleDelete}
                disabled={busy}
                className="text-destructive hover:text-destructive"
              >
                <Trash2 className="h-4 w-4" /> Delete
              </Button>
            )}
          </div>
          <div className="flex items-center gap-2">
            <Button
              type="button"
              variant="outline"
              onClick={handleTest}
              disabled={busy}
            >
              {testConnection.isPending ? (
                <Loader2 className="h-4 w-4 animate-spin" />
              ) : (
                <Plug className="h-4 w-4" />
              )}
              Test
            </Button>
            {existing && (
              <Button
                type="button"
                onClick={handleConnect}
                disabled={busy}
                className="bg-primary"
              >
                {openTerminal.isPending ? (
                  <Loader2 className="h-4 w-4 animate-spin" />
                ) : (
                  <Plug className="h-4 w-4" />
                )}
                Connect & Open Terminal
              </Button>
            )}
            <Button type="button" onClick={handleSave} disabled={busy}>
              {busy && !testConnection.isPending && !openTerminal.isPending ? (
                <Loader2 className="h-4 w-4 animate-spin" />
              ) : (
                <Save className="h-4 w-4" />
              )}
              {existing ? "Save" : "Create"}
            </Button>
          </div>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
