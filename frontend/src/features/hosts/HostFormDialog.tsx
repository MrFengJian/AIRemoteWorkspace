import { useEffect, useRef, useState } from "react";
import { Loader2, Plug, Save, Trash2, KeyRound } from "lucide-react";

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
import { Checkbox } from "@/components/ui/checkbox";
import {
  AUTH_TYPES,
  AUTH_TYPE_LABELS,
  hostsApi,
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
  authType: "password",
  keyPath: "",
};

const EMPTY_CREDS: CredentialsDTO = { password: "", keyPath: "", keyPassphrase: "", useAgent: false };

/**
 * HostFormDialog — create/edit/delete + test-connect + open-terminal, with
 * optional "remember password" via the OS credential vault.
 *
 * Credentials live in component state only; when "remember" is checked the
 * secret is written to the OS keychain (never to SQLite). When editing a host
 * that already has a remembered secret, the field shows a placeholder and the
 * checkbox pre-checks; the actual value is read from the vault at connect time.
 */
export function HostFormDialog() {
  const editing = useHostsUIStore((s) => s.editing);
  const closeEditor = useHostsUIStore((s) => s.closeEditor);
  const openEditor = useHostsUIStore((s) => s.openEditor);
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
  const [remember, setRemember] = useState(false);
  const [hasRemembered, setHasRemembered] = useState(false);

  // Guards against duplicate submits: a form Enter-key submit + button click,
  // or a fast double-click, can both fire mutateAsync before isPending flips.
  const submitting = useRef(false);

  // Sync local form state when the dialog target changes.
  useEffect(() => {
    if (!isOpen) return;
    if (existing) {
      setInput({
        name: existing.name,
        host: existing.host,
        port: existing.port,
        username: existing.username,
        authType: (existing.authType || "password") as AuthType,
        keyPath: "",
      });
      // Check if a remembered secret already exists for this host.
      hostsApi
        .getRemembered(existing.id)
        .then((info) => {
          const remembered =
            (input.authType === "password" && info.hasPassword) ||
            (input.authType === "key" && info.hasPassphrase);
          setHasRemembered(remembered);
          setRemember(remembered);
        })
        .catch(() => setHasRemembered(false));
    } else {
      setInput(EMPTY_INPUT);
      setHasRemembered(false);
      setRemember(false);
    }
    setCreds(EMPTY_CREDS);
    setTestResult(null);
    // eslint-disable-next-line react-hooks/exhaustive-deps
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

  const persistRemembered = async (hostId: string) => {
    try {
      await hostsApi.saveCredentials(hostId, buildCreds(), remember);
    } catch {
      /* non-fatal: vault may be unavailable; the connect still happened */
    }
  };

  const handleSave = async () => {
    if (submitting.current) return;
    submitting.current = true;
    try {
      const saved = existing
        ? await updateHost.mutateAsync({ id: existing.id, input })
        : await createHost.mutateAsync(input);
      if (remember) {
        await persistRemembered(saved.id);
      } else if (existing) {
        // clearing is meaningful only on an existing host
        await hostsApi.saveCredentials(existing.id, EMPTY_CREDS, false).catch(() => {});
      }
      closeEditor();
    } catch {
      /* mutation error surfaces via the hook's state; keep dialog open */
    } finally {
      submitting.current = false;
    }
  };

  const handleDelete = async () => {
    if (!existing) return;
    if (!confirm(`Delete host "${existing.name}"? This also clears any remembered password.`)) return;
    await deleteHost.mutateAsync(existing.id);
    closeEditor();
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

  const handleTest = async () => {
    if (submitting.current) return;
    submitting.current = true;
    try {
      let hostId = existing?.id;
      if (!hostId) {
        // For an unsaved host, persist first so we have an id to test against.
        // Then switch the dialog to edit mode so a later "Save" updates this
        // host instead of creating a duplicate.
        const created = await createHost.mutateAsync(input);
        hostId = created.id;
        openEditor(created);
      }
      await runTest(hostId);
    } finally {
      submitting.current = false;
    }
  };

  const handleConnect = async () => {
    if (!existing) return;
    try {
      // ResolveCredentials on the backend fills in remembered secrets when the
      // frontend sends blanks, so passing an empty password still connects.
      await openTerminal.mutateAsync({
        host: { id: existing.id, name: existing.name },
        creds: buildCreds(),
      });
      if (remember) await persistRemembered(existing.id);
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

  // Placeholder text for the secret field depends on whether a remembered
  // secret exists (we never reveal the stored value).
  const secretPlaceholder = hasRemembered
    ? "•••••••• (saved in OS vault — leave blank to reuse)"
    : undefined;

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
            session unless you tick “Remember” — then they’re stored in the OS
            credential vault (Windows Credential Manager / macOS Keychain /
            Linux Secret Service), never in the database.
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

          {/* Credentials — session only, optionally persisted to OS vault. */}
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
                  placeholder={secretPlaceholder}
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
                    placeholder={secretPlaceholder}
                  />
                </div>
              </>
            )}
            {input.authType === "agent" && (
              <p className="text-xs text-muted-foreground">
                Uses ssh-agent via SSH_AUTH_SOCK. No credential entry needed.
              </p>
            )}

            {/* Remember toggle — stores the secret in the OS credential vault. */}
            {input.authType !== "agent" && (
              <label className="flex cursor-pointer items-center gap-2 pt-1 text-sm">
                <Checkbox
                  checked={remember}
                  onCheckedChange={(v) => setRemember(v === true)}
                />
                <span className="flex items-center gap-1.5">
                  <KeyRound className="h-3.5 w-3.5 text-muted-foreground" />
                  {input.authType === "password" ? "Remember password" : "Remember passphrase"}
                  {hasRemembered && (
                    <Badge variant="success" className="ml-1 text-[10px]">saved</Badge>
                  )}
                </span>
              </label>
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
