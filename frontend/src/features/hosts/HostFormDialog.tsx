import { useEffect, useRef, useState } from "react";
import { Loader2, Plug, Save, Trash2, KeyRound, Network } from "lucide-react";
import { useTranslation } from "react-i18next";

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
  HOST_GROUPS,
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
import { TERMINAL_THEMES, getTerminalTheme } from "@/features/terminal/themes";
import { TERMINAL_FONTS, terminalFontFamily } from "@/features/terminal/fonts";
import { osInfo } from "@/features/hosts/osIcons";
import {
  EMPTY_TUNNEL,
  TUNNEL_TYPES,
  tunnelApi,
  type TunnelConfig,
} from "@/features/tunnel/api";
import { useConfirm } from "@/lib/useConfirm";
import { cn } from "@/lib/utils";

const EMPTY_INPUT: HostInputDTO = {
  name: "",
  host: "",
  port: 22,
  username: "",
  authType: "password",
  keyPath: "",
  terminalTheme: "",
  terminalFont: "",
  terminalFontSize: 0,
  group: "",
  tags: [],
  tunnels: [],
};

const EMPTY_CREDS: CredentialsDTO = { password: "", keyPath: "", keyPassphrase: "", useAgent: false };

/** Dialog tab groups, Xshell-style: connection / appearance / organisation / tunnel. */
type FormTab = "connection" | "appearance" | "organisation" | "tunnel";

/**
 * HostFormDialog — create/edit/delete + test-connect + open-terminal, with
 * optional "remember password" via the OS credential vault. Fields are
 * grouped into tabs: 连接 (connection + credentials), 外观 (terminal colour
 * scheme + font, per-host), 组织 (group + tags).
 *
 * Credentials live in component state only; when "remember" is checked the
 * secret is written to the OS keychain (never to SQLite). When editing a host
 * that already has a remembered secret, the field shows a placeholder and the
 * checkbox pre-checks; the actual value is read from the vault at connect time.
 */
export function HostFormDialog() {
  const { t } = useTranslation();
  const editing = useHostsUIStore((s) => s.editing);
  const closeEditor = useHostsUIStore((s) => s.closeEditor);
  const openEditor = useHostsUIStore((s) => s.openEditor);
  const { askConfirm } = useConfirm();

  const createHost = useCreateHost();
  const updateHost = useUpdateHost();
  const deleteHost = useDeleteHost();
  const testConnection = useTestConnection();
  const openTerminal = useOpenTerminal();

  const isOpen = editing !== null;
  const existing: HostDTO | null = editing && editing !== "new" ? editing : null;

  const [tab, setTab] = useState<FormTab>("connection");
  const [input, setInput] = useState<HostInputDTO>(EMPTY_INPUT);
  const [creds, setCreds] = useState<CredentialsDTO>(EMPTY_CREDS);
  const [testResult, setTestResult] = useState<{ ok: boolean; msg: string } | null>(null);
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [remember, setRemember] = useState(false);
  const [hasRemembered, setHasRemembered] = useState(false);

  // Guards against duplicate submits: a form Enter-key submit + button click,
  // or a fast double-click, can both fire mutateAsync before isPending flips.
  const submitting = useRef(false);

  // Sync local form state when the dialog target changes.
  useEffect(() => {
    if (!isOpen) return;
    setTab("connection");
    if (existing) {
      setInput({
        name: existing.name,
        host: existing.host,
        port: existing.port,
        username: existing.username,
        authType: (existing.authType || "password") as AuthType,
        keyPath: "",
        terminalTheme: existing.terminalTheme || "",
        terminalFont: existing.terminalFont || "",
        terminalFontSize: existing.terminalFontSize || 0,
        group: existing.group || "",
        tags: existing.tags ?? [],
        tunnels: existing.tunnels ?? [],
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
    setErrors({});
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [isOpen, existing]);

  const update = (patch: Partial<HostInputDTO>) => {
    setInput((v) => ({ ...v, ...patch }));
    // Clear the field's error as soon as the user edits it again.
    setErrors((e) => {
      if (Object.keys(e).length === 0) return e;
      const next = { ...e };
      for (const key of Object.keys(patch)) delete next[key];
      return next;
    });
  };
  const updateCreds = (patch: Partial<CredentialsDTO>) =>
    setCreds((v) => ({ ...v, ...patch }));

  // ── Tunnel rule list helpers ────────────────────────────────────────
  const updateRule = (i: number, patch: Partial<TunnelConfig>) => {
    update({
      tunnels: (input.tunnels ?? []).map((r, j) => (j === i ? { ...r, ...patch } : r)),
    });
  };
  const addRule = () => {
    update({ tunnels: [...(input.tunnels ?? []), { ...EMPTY_TUNNEL }] });
  };
  const removeRule = (i: number) => {
    update({ tunnels: (input.tunnels ?? []).filter((_, j) => j !== i) });
  };

  const buildCreds = (): CredentialsDTO => ({
    password: input.authType === "password" ? creds.password : "",
    keyPath: input.authType === "key" ? (creds.keyPath || input.keyPath) : "",
    keyPassphrase: input.authType === "key" ? creds.keyPassphrase : "",
    useAgent: input.authType === "agent",
  });

  /** Field-level validation. Returns a map of field → error message. */
  const validate = (): Record<string, string> => {
    const errs: Record<string, string> = {};
    if (!input.name.trim()) errs.name = t("hostForm.errRequired");
    if (!input.host.trim()) errs.host = t("hostForm.errRequired");
    if (!input.username.trim()) errs.username = t("hostForm.errRequired");
    if (!Number.isInteger(input.port) || input.port < 1 || input.port > 65535) {
      errs.port = t("hostForm.errPort");
    }
    // Tunnel rules only bind when the rule is enabled.
    const seenLocalPorts = new Set<number>();
    (input.tunnels ?? []).forEach((tun, i) => {
      if (!tun.enabled) return;
      const tag = (k: string) => `tunnel-${i}-${k}`;
      if (!Number.isInteger(tun.listenPort) || tun.listenPort < 1 || tun.listenPort > 65535) {
        errs[tag("listenPort")] = t("hostForm.errPort");
      }
      if (tun.type === "local" || tun.type === "remote") {
        if (!tun.targetHost?.trim()) errs[tag("targetHost")] = t("hostForm.errRequired");
        if (
          typeof tun.targetPort !== "number" ||
          !Number.isInteger(tun.targetPort) ||
          tun.targetPort < 1 ||
          tun.targetPort > 65535
        ) {
          errs[tag("targetPort")] = t("hostForm.errPort");
        }
      }
      // Two enabled LOCAL-side listeners (local/dynamic) on the same port can
      // never coexist — flag it directly instead of prompting at save.
      if (tun.type !== "remote" && Number.isInteger(tun.listenPort) && tun.listenPort > 0) {
        if (seenLocalPorts.has(tun.listenPort)) {
          errs[tag("listenPort")] = t("hostForm.errTunnelDupPort");
        }
        seenLocalPorts.add(tun.listenPort);
      }
    });
    return errs;
  };

  /** Validate and surface errors, switching to the tab that owns them. */
  const validateOrSwitch = (): boolean => {
    const errs = validate();
    if (Object.keys(errs).length > 0) {
      setErrors(errs);
      const hasTunnelErr = Object.keys(errs).some((k) => k.startsWith("tunnel"));
      setTab(hasTunnelErr ? "tunnel" : "connection");
      return false;
    }
    return true;
  };

  const persistRemembered = async (hostId: string) => {
    try {
      await hostsApi.saveCredentials(hostId, buildCreds(), remember);
    } catch {
      /* non-fatal: vault may be unavailable; the connect still happened */
    }
  };

  const handleSave = async () => {
    if (submitting.current) return;
    if (!validateOrSwitch()) return;
    submitting.current = true;
    try {
      // Save-time port conflict pre-check: enabled LOCAL-side listeners
      // (local/dynamic rules) must not collide with ports already held by
      // other processes or other hosts' tunnels. Ports belonging to this
      // host's own tunnels are exempt on the backend. Prompting, not
      // blocking — the user may know better.
      const ports = (input.tunnels ?? [])
        .filter((r) => r.enabled && r.type !== "remote" && r.listenPort > 0)
        .map((r) => r.listenPort);
      if (ports.length > 0) {
        const busy = await tunnelApi.checkPorts(existing?.id ?? "", ports);
        if (busy.length > 0) {
          const ok = await askConfirm({
            title: t("hostForm.tunnelPortConflictTitle"),
            message: t("hostForm.tunnelPortConflict", { ports: busy.join(", ") }),
            confirmLabel: t("common.save"),
          });
          if (!ok) return;
        }
      }
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
      /* keep the dialog open; the failure is toasted globally */
    } finally {
      submitting.current = false;
    }
  };

  const handleDelete = async () => {
    if (!existing) return;
    const ok = await askConfirm({
      title: t("hosts.deleteTitle"),
      message: t("hosts.deleteConfirm", { name: existing.name }),
      danger: true,
      confirmLabel: t("common.delete"),
    });
    if (!ok) return;
    await deleteHost.mutateAsync(existing.id).catch(() => {});
    closeEditor();
  };

  const runTest = async (hostId: string) => {
    setTestResult(null);
    try {
      const res = await testConnection.mutateAsync({
        hostId,
        creds: buildCreds(),
      });
      setTestResult(
        res.ok
          ? { ok: true, msg: t("hostForm.testConnected") }
          : { ok: false, msg: res.msg },
      );
    } catch (e) {
      setTestResult({ ok: false, msg: e instanceof Error ? e.message : String(e) });
    }
  };

  const handleTest = async () => {
    if (submitting.current) return;
    if (!validateOrSwitch()) return;
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
    } catch {
      /* creation failure is toasted globally */
    } finally {
      submitting.current = false;
    }
  };

  const handleConnect = async () => {
    if (!existing) return;
    if (!validateOrSwitch()) return;
    try {
      // ResolveCredentials on the backend fills in remembered secrets when the
      // frontend sends blanks, so passing an empty password still connects.
      await openTerminal.mutateAsync({
        host: {
          id: existing.id,
          name: existing.name,
          terminalTheme: input.terminalTheme,
          terminalFont: input.terminalFont,
          terminalFontSize: input.terminalFontSize,
        },
        creds: buildCreds(),
      });
      if (remember) await persistRemembered(existing.id);
      closeEditor();
    } catch {
      /* keep the dialog open; the failure is toasted globally */
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
    ? t("hostForm.savedSecretPlaceholder")
    : undefined;

  // Appearance tab: resolve the theme ("" = default scheme) for the preview.
  const previewTheme = getTerminalTheme(input.terminalTheme).theme;

  const tabs: { id: FormTab; label: string }[] = [
    { id: "connection", label: t("hostForm.connection") },
    { id: "appearance", label: t("hostForm.appearance") },
    { id: "organisation", label: t("hostForm.organisation") },
    { id: "tunnel", label: t("hostForm.tunnel") },
  ];

  return (
    <Dialog
      open={isOpen}
      onOpenChange={(o) => {
        if (!o) closeEditor();
      }}
    >
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>{existing ? t("hostForm.editTitle") : t("hostForm.addTitle")}</DialogTitle>
          <DialogDescription>{t("hostForm.description")}</DialogDescription>
        </DialogHeader>

        <form
          className="flex min-h-0 flex-col gap-4"
          onSubmit={(e) => {
            e.preventDefault();
            handleSave();
          }}
        >
          {/* Tab bar — Xshell-style field grouping */}
          <div
            role="tablist"
            aria-label={t("hostForm.editTitle")}
            className="flex gap-1 border-b border-border"
          >
            {tabs.map((tb) => (
              <button
                key={tb.id}
                type="button"
                role="tab"
                id={`host-form-tab-${tb.id}`}
                aria-selected={tab === tb.id}
                aria-controls={`host-form-panel-${tb.id}`}
                tabIndex={tab === tb.id ? 0 : -1}
                onClick={() => setTab(tb.id)}
                className={cn(
                  "-mb-px rounded-t-[var(--radius)] border-b-2 px-3 py-1.5 text-sm transition-colors",
                  "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
                  tab === tb.id
                    ? "border-primary font-medium text-foreground"
                    : "border-transparent text-muted-foreground hover:text-foreground",
                )}
              >
                {tb.label}
              </button>
            ))}
          </div>

          {/* ── Tab: Connection (info + credentials) ─────────────────── */}
          {tab === "connection" && (
            <div
              role="tabpanel"
              id="host-form-panel-connection"
              aria-labelledby="host-form-tab-connection"
              className="flex flex-col gap-3"
            >
              {/* Read-only OS indicator (auto-detected at connect time). */}
              {existing && osInfo(existing.os) && (
                <div className="flex items-center gap-2 rounded-[var(--radius)] bg-muted/30 px-2.5 py-1.5 text-xs text-muted-foreground">
                  <img
                    src={osInfo(existing.os)!.icon}
                    alt=""
                    className="os-icon h-3.5 w-3.5"
                  />
                  {osInfo(existing.os)!.label}
                  <span className="ml-auto text-[10px] uppercase tracking-wide opacity-60">
                    {t("hostForm.osAuto")}
                  </span>
                </div>
              )}

              <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
                <div className="grid gap-1.5">
                  <Label htmlFor="name">{t("hostForm.name")}</Label>
                  <Input
                    id="name"
                    value={input.name}
                    onChange={(e) => update({ name: e.target.value })}
                    placeholder="production-web-1"
                    aria-invalid={!!errors.name}
                  />
                  {errors.name && <FieldError message={errors.name} />}
                </div>

                <div className="grid gap-1.5">
                  <Label htmlFor="host">{t("hostForm.host")}</Label>
                  <Input
                    id="host"
                    value={input.host}
                    onChange={(e) => update({ host: e.target.value })}
                    placeholder="10.0.0.1 or example.com"
                    aria-invalid={!!errors.host}
                  />
                  {errors.host && <FieldError message={errors.host} />}
                </div>

                <div className="grid gap-1.5">
                  <Label htmlFor="username">{t("hostForm.username")}</Label>
                  <Input
                    id="username"
                    value={input.username}
                    onChange={(e) => update({ username: e.target.value })}
                    placeholder="root"
                    aria-invalid={!!errors.username}
                  />
                  {errors.username && <FieldError message={errors.username} />}
                </div>

                <div className="grid gap-1.5">
                  <Label htmlFor="port">{t("hostForm.port")}</Label>
                  <Input
                    id="port"
                    type="number"
                    min={1}
                    max={65535}
                    value={input.port}
                    onChange={(e) => update({ port: Number(e.target.value) })}
                    aria-invalid={!!errors.port}
                  />
                  {errors.port && <FieldError message={errors.port} />}
                </div>
              </div>

              <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
                <div className="grid gap-1.5">
                  <Label htmlFor="authType">{t("hostForm.authentication")}</Label>
                  <Select
                    id="authType"
                    value={input.authType}
                    onChange={(e) => update({ authType: e.target.value as AuthType })}
                  >
                    {AUTH_TYPES.map((at) => (
                      <option key={at} value={at}>
                        {t(`hosts.authType.${at}`)}
                      </option>
                    ))}
                  </Select>
                </div>

                {input.authType === "key" && (
                  <div className="grid gap-1.5">
                    <Label htmlFor="keyPath">{t("hostForm.keyPath")}</Label>
                    <Input
                      id="keyPath"
                      value={input.keyPath}
                      onChange={(e) => update({ keyPath: e.target.value })}
                      placeholder="C:\Users\you\.ssh\id_ed25519"
                    />
                  </div>
                )}
              </div>

              {/* Credentials sub-section */}
              <div className="flex flex-col gap-3 rounded-[var(--radius)] border border-border bg-background/30 p-3">
                <div className="flex items-center gap-1.5 text-xs font-semibold uppercase tracking-wide text-muted-foreground">
                  <KeyRound className="h-3.5 w-3.5" /> {t("hostForm.credentials")}
                </div>

                {input.authType === "password" && (
                  <div className="grid gap-1.5">
                    <Label htmlFor="password">{t("hostForm.password")}</Label>
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
                  <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
                    <div className="grid gap-1.5">
                      <Label htmlFor="credsKeyPath">{t("hostForm.keyOverride")}</Label>
                      <Input
                        id="credsKeyPath"
                        value={creds.keyPath}
                        onChange={(e) => updateCreds({ keyPath: e.target.value })}
                        placeholder={input.keyPath || "path to private key"}
                      />
                    </div>
                    <div className="grid gap-1.5">
                      <Label htmlFor="passphrase">{t("hostForm.passphrase")}</Label>
                      <Input
                        id="passphrase"
                        type="password"
                        value={creds.keyPassphrase}
                        onChange={(e) => updateCreds({ keyPassphrase: e.target.value })}
                        placeholder={secretPlaceholder}
                      />
                    </div>
                  </div>
                )}
                {input.authType === "agent" && (
                  <p className="text-xs text-muted-foreground">
                    {t("hostForm.agentHint")}
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
                      {input.authType === "password" ? t("hostForm.rememberPassword") : t("hostForm.rememberPassphrase")}
                      {hasRemembered && (
                        <Badge variant="success" className="ml-1 text-[10px]">{t("hostForm.saved")}</Badge>
                      )}
                    </span>
                  </label>
                )}

                {testResult && (
                  <div className="flex items-center gap-2">
                    <Badge variant={testResult.ok ? "success" : "destructive"}>
                      {testResult.ok ? t("hostForm.testOk") : t("hostForm.testFailed")}
                    </Badge>
                    {!testResult.ok && testResult.msg && (
                      <span className="min-w-0 flex-1 break-all text-xs text-muted-foreground">
                        {testResult.msg}
                      </span>
                    )}
                  </div>
                )}
              </div>
            </div>
          )}

          {/* ── Tab: Appearance (terminal scheme + font, per-host) ──── */}
          {tab === "appearance" && (
            <div
              role="tabpanel"
              id="host-form-panel-appearance"
              aria-labelledby="host-form-tab-appearance"
              className="flex flex-col gap-4"
            >
              {/* Terminal colour scheme */}
              <div className="grid gap-1.5">
                <Label htmlFor="termTheme">{t("hostForm.terminalScheme")}</Label>
                <div className="flex items-center gap-2">
                  <Select
                    id="termTheme"
                    value={input.terminalTheme || ""}
                    onChange={(e) => update({ terminalTheme: e.target.value })}
                    className="min-w-0 flex-1"
                  >
                    <option value="">{t("hostForm.schemeDefault")}</option>
                    {TERMINAL_THEMES.map((th) => (
                      <option key={th.id} value={th.id}>
                        {th.label}
                      </option>
                    ))}
                  </Select>
                  {input.terminalTheme && (
                    <div
                      className="flex h-9 w-16 shrink-0 items-center justify-center gap-1 rounded-[var(--radius)] border border-border font-mono text-xs"
                      style={{
                        background: previewTheme.background,
                      }}
                    >
                      <span style={{ color: previewTheme.red }}>●</span>
                      <span style={{ color: previewTheme.green }}>●</span>
                      <span style={{ color: previewTheme.blue }}>●</span>
                    </div>
                  )}
                </div>
              </div>

              {/* Terminal font */}
              <div className="grid gap-1.5">
                <Label htmlFor="termFont">{t("hostForm.termFont")}</Label>
                <Select
                  id="termFont"
                  value={input.terminalFont}
                  onChange={(e) => update({ terminalFont: e.target.value })}
                >
                  {TERMINAL_FONTS.map((f) => (
                    <option key={f.value} value={f.value}>{f.label}</option>
                  ))}
                </Select>
              </div>

              {/* Terminal font size */}
              <div className="grid gap-1.5">
                <Label htmlFor="termFontSize">{t("hostForm.termFontSize")}</Label>
                <div className="flex items-center gap-3">
                  <input
                    id="termFontSize"
                    type="range"
                    min={10}
                    max={22}
                    value={input.terminalFontSize || 13}
                    onChange={(e) => update({ terminalFontSize: Number(e.target.value) })}
                    className="flex-1 accent-primary"
                  />
                  <span className="w-10 text-center font-mono text-xs">
                    {input.terminalFontSize || 13}px
                  </span>
                </div>
              </div>

              {/* Live preview: selected theme colours + font + size. */}
              <div className="grid gap-1.5">
                <span className="text-[10px] uppercase tracking-wide text-muted-foreground">
                  {t("termAppearanceDialog.preview")}
                </span>
                <div
                  className="overflow-hidden whitespace-nowrap rounded-[var(--radius)] border border-border px-3 py-2"
                  style={{
                    background: previewTheme.background,
                    fontFamily: terminalFontFamily(input.terminalFont),
                    fontSize: `${input.terminalFontSize || 13}px`,
                    lineHeight: 1.5,
                  }}
                >
                  <span style={{ color: previewTheme.green }}>user@host</span>
                  <span style={{ color: previewTheme.foreground }}>:~$ </span>
                  <span style={{ color: previewTheme.blue }}>df</span>
                  <span style={{ color: previewTheme.foreground }}> -h</span>
                  <span
                    className="ml-0.5 inline-block h-[1em] w-[0.55em] translate-y-[0.15em] animate-pulse"
                    style={{ background: previewTheme.cursor ?? previewTheme.foreground }}
                  />
                </div>
              </div>
            </div>
          )}

          {/* ── Tab: Organisation (group + tags) ─────────────────────── */}
          {tab === "organisation" && (
            <div
              role="tabpanel"
              id="host-form-panel-organisation"
              aria-labelledby="host-form-tab-organisation"
              className="flex flex-col gap-4"
            >
              <div className="grid gap-1.5">
                <Label htmlFor="group">{t("hostForm.group")}</Label>
                <Select
                  id="group"
                  value={isPresetGroup(input.group) ? input.group : "custom"}
                  onChange={(e) => {
                    const v = e.target.value;
                    // Switching to a preset clears any custom name; picking
                    // "custom" keeps the current custom name (or empty).
                    update({ group: v === "custom" ? (isPresetGroup(input.group) ? "" : input.group) : v });
                  }}
                >
                  {HOST_GROUPS.map((g) => (
                    <option key={g} value={g}>
                      {g === "" ? t("hosts.group.none") : g}
                    </option>
                  ))}
                  <option value="custom">{t("hostForm.customGroup")}</option>
                </Select>
              </div>

              {/* Custom group name — only shown when "Custom group" is selected,
                  so typing here can never be silently discarded. */}
              {!isPresetGroup(input.group) && (
                <div className="grid gap-1.5">
                  <Label htmlFor="customGroup">{t("hostForm.customGroup")}</Label>
                  <Input
                    id="customGroup"
                    value={input.group}
                    onChange={(e) => update({ group: e.target.value })}
                    placeholder="e.g. dev-cluster"
                  />
                </div>
              )}

              {/* Tags */}
              <div className="grid gap-1.5">
                <Label>{t("hostForm.tags")}</Label>
                <div className="flex flex-wrap items-center gap-1.5">
                  {(input.tags ?? []).map((tag, i) => (
                    <span
                      key={`${tag}-${i}`}
                      className="inline-flex items-center gap-1 rounded-full border border-primary/40 bg-primary/10 px-2 py-0.5 text-xs"
                    >
                      {tag}
                      <button
                        type="button"
                        onClick={() =>
                          update({ tags: (input.tags ?? []).filter((_, j) => j !== i) })
                        }
                        className="text-muted-foreground hover:text-foreground"
                        aria-label={`Remove tag ${tag}`}
                      >
                        ✕
                      </button>
                    </span>
                  ))}
                </div>
                <TagInput
                  onAdd={(tag) => {
                    const current = input.tags ?? [];
                    if (tag && !current.includes(tag)) {
                      update({ tags: [...current, tag] });
                    }
                  }}
                />
              </div>
            </div>
          )}

          {/* ── Tab: Tunnel (SSH port forwarding rules per host) ─────── */}
          {tab === "tunnel" && (
            <div
              role="tabpanel"
              id="host-form-panel-tunnel"
              aria-labelledby="host-form-tab-tunnel"
              className="flex flex-col gap-3"
            >
              <p className="flex items-start gap-1.5 rounded-[var(--radius)] bg-muted/30 px-2.5 py-2 text-xs text-muted-foreground">
                <Network className="mt-0.5 h-3.5 w-3.5 shrink-0" />
                {t("hostForm.tunnelHint")}
              </p>

              {(input.tunnels ?? []).length === 0 && (
                <p className="py-2 text-center text-xs text-muted-foreground">
                  {t("hostForm.tunnelNone")}
                </p>
              )}

              {(input.tunnels ?? []).map((rule, i) => {
                const tag = (k: string) => `tunnel-${i}-${k}`;
                return (
                  <div
                    key={i}
                    className={cn(
                      "flex flex-col gap-3 rounded-[var(--radius)] border p-3",
                      rule.enabled ? "border-border bg-background/30" : "border-dashed border-border/60 opacity-70",
                    )}
                  >
                    {/* Rule header: enable toggle + type + remove */}
                    <div className="flex items-center gap-2">
                      <Checkbox
                        checked={rule.enabled}
                        onCheckedChange={(v) => updateRule(i, { enabled: v === true })}
                        aria-label={t("hostForm.tunnelRuleEnable")}
                      />
                      <span className="text-xs font-semibold text-foreground">
                        {t("hostForm.tunnelRuleN", { n: i + 1 })}
                      </span>
                      <span className="min-w-0 flex-1 truncate font-mono text-[11px] text-muted-foreground">
                        {t(`hostForm.tunnelType_${rule.type}`)}
                      </span>
                      <Button
                        type="button"
                        variant="ghost"
                        size="icon"
                        className="h-6 w-6 text-muted-foreground hover:text-destructive"
                        onClick={() => removeRule(i)}
                        aria-label={t("hostForm.tunnelRuleDelete")}
                        title={t("hostForm.tunnelRuleDelete")}
                      >
                        <Trash2 className="h-3.5 w-3.5" />
                      </Button>
                    </div>

                    <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
                      <div className="grid gap-1.5">
                        <Label htmlFor={`tunnelType-${i}`}>{t("hostForm.tunnelType")}</Label>
                        <Select
                          id={`tunnelType-${i}`}
                          value={rule.type}
                          onChange={(e) =>
                            updateRule(i, {
                              type: e.target.value as (typeof TUNNEL_TYPES)[number],
                            })
                          }
                        >
                          {TUNNEL_TYPES.map((tt) => (
                            <option key={tt} value={tt}>
                              {t(`hostForm.tunnelType_${tt}`)}
                            </option>
                          ))}
                        </Select>
                      </div>

                      <div className="grid gap-1.5">
                        <Label htmlFor={`tunnelBind-${i}`}>{t("hostForm.tunnelBindHost")}</Label>
                        <Select
                          id={`tunnelBind-${i}`}
                          value={rule.bindHost || "127.0.0.1"}
                          onChange={(e) => updateRule(i, { bindHost: e.target.value })}
                        >
                          <option value="127.0.0.1">{t("hostForm.tunnelBindLoopback")}</option>
                          <option value="0.0.0.0">{t("hostForm.tunnelBindAll")}</option>
                        </Select>
                      </div>

                      <div className="grid gap-1.5">
                        <Label htmlFor={`tunnelListenPort-${i}`}>
                          {rule.type === "remote"
                            ? t("hostForm.tunnelListenPortRemote")
                            : t("hostForm.tunnelListenPort")}
                        </Label>
                        <Input
                          id={`tunnelListenPort-${i}`}
                          type="number"
                          min={1}
                          max={65535}
                          value={rule.listenPort || ""}
                          onChange={(e) => updateRule(i, { listenPort: Number(e.target.value) })}
                          placeholder={rule.type === "remote" ? "8080" : "13306"}
                          aria-invalid={!!errors[tag("listenPort")]}
                        />
                        {errors[tag("listenPort")] && (
                          <FieldError message={errors[tag("listenPort")]} />
                        )}
                      </div>
                    </div>

                    {rule.type !== "dynamic" && (
                      <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
                        <div className="grid gap-1.5">
                          <Label htmlFor={`tunnelTargetHost-${i}`}>
                            {t("hostForm.tunnelTargetHost")}
                            <span className="ml-1 text-[10px] font-normal text-muted-foreground">
                              {rule.type === "remote"
                                ? t("hostForm.tunnelTargetFromLocal")
                                : t("hostForm.tunnelTargetFromServer")}
                            </span>
                          </Label>
                          <Input
                            id={`tunnelTargetHost-${i}`}
                            value={rule.targetHost ?? ""}
                            onChange={(e) => updateRule(i, { targetHost: e.target.value })}
                            placeholder="127.0.0.1"
                            aria-invalid={!!errors[tag("targetHost")]}
                          />
                          {errors[tag("targetHost")] && (
                            <FieldError message={errors[tag("targetHost")]} />
                          )}
                        </div>
                        <div className="grid gap-1.5">
                          <Label htmlFor={`tunnelTargetPort-${i}`}>
                            {t("hostForm.tunnelTargetPort")}
                          </Label>
                          <Input
                            id={`tunnelTargetPort-${i}`}
                            type="number"
                            min={1}
                            max={65535}
                            value={rule.targetPort || ""}
                            onChange={(e) => updateRule(i, { targetPort: Number(e.target.value) })}
                            placeholder="3306"
                            aria-invalid={!!errors[tag("targetPort")]}
                          />
                          {errors[tag("targetPort")] && (
                            <FieldError message={errors[tag("targetPort")]} />
                          )}
                        </div>
                      </div>
                    )}
                  </div>
                );
              })}

              <Button
                type="button"
                variant="outline"
                size="sm"
                className="w-fit"
                onClick={addRule}
              >
                <Network className="h-3.5 w-3.5" /> {t("hostForm.tunnelAdd")}
              </Button>
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
                <Trash2 className="h-4 w-4" /> {t("hostForm.deleteHost")}
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
              {t("hostForm.test")}
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
                {t("hostForm.connectOpen")}
              </Button>
            )}
            <Button type="button" onClick={handleSave} disabled={busy}>
              {busy && !testConnection.isPending && !openTerminal.isPending ? (
                <Loader2 className="h-4 w-4 animate-spin" />
              ) : (
                <Save className="h-4 w-4" />
              )}
              {existing ? t("common.save") : t("common.create")}
            </Button>
          </div>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

/** True when the group is one of the preset options (including "none"). */
function isPresetGroup(group: string): boolean {
  return HOST_GROUPS.includes(group as (typeof HOST_GROUPS)[number]);
}

/** Inline validation error shown under a form field. */
function FieldError({ message }: { message: string }) {
  return <p className="text-xs text-destructive">{message}</p>;
}

/** TagInput — type a tag, press Enter or comma to add it. */
function TagInput({ onAdd }: { onAdd: (tag: string) => void }) {
  const { t } = useTranslation();
  const [val, setVal] = useState("");

  const commit = () => {
    const tag = val.trim().replace(/,+$/, "");
    if (tag) onAdd(tag);
    setVal("");
  };

  return (
    <Input
      value={val}
      onChange={(e) => setVal(e.target.value)}
      onKeyDown={(e) => {
        if (e.key === "Enter" || e.key === ",") {
          e.preventDefault();
          commit();
        }
        if (e.key === "Backspace" && val === "") {
          // handled by the chips' remove buttons; no-op here
        }
      }}
      onBlur={commit}
      placeholder={t("hostForm.tagPlaceholder")}
      className="h-8 text-xs"
    />
  );
}
