import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Events } from "@wailsio/runtime";
import { Check, CircleAlert, Loader2, Pencil, RotateCw } from "lucide-react";

import type { HostDTO } from "@/features/hosts/api";
import { HOSTS_KEY } from "@/features/hosts/hooks";import { useUIStore } from "@/stores/ui.store";
import { useHostsUIStore } from "@/features/hosts/store";
import { queryClient } from "@/lib/queryClient";
import {
  CONNECT_STAGES,
  classifyConnectError,
  useConnectStore,
  type ConnectStage,
} from "@/features/hosts/connect.store";
import { cn } from "@/lib/utils";

/** Stage progress label keys, in backend order. */
const STAGE_LABELS: Record<ConnectStage, string> = {
  credentials: "connect.stage_credentials",
  connect: "connect.stage_connect",
  handshake: "connect.stage_handshake",
  session: "connect.stage_session",
};

/**
 * Global SSH connection overlay: while a terminal tab is being opened it
 * shows a prominent staged progress card (host name + stage checklist +
 * elapsed seconds); on failure it turns into a dialog with the classified
 * cause, the raw error, and retry / edit-host actions. Mounted once in
 * AppShell; the useOpenTerminal mutation drives the store.
 */
export function ConnectOverlay() {
  const { t } = useTranslation();
  const state = useConnectStore();
  const setView = useUIStore((s) => s.setView);
  const openEditor = useHostsUIStore((s) => s.openEditor);
  const [elapsed, setElapsed] = useState(0);

  // Progress events: backend emits "terminal:connect" {connectId, stage}
  // while the OpenSession call is in flight.
  useEffect(() => {
    const cancel = Events.On("terminal:connect", (e: unknown) => {
      const data = (e as { data?: { connectId?: string; stage?: ConnectStage } }).data;
      if (data?.connectId && data.stage) state.setStage(data.connectId, data.stage);
    });
    return () => {
      if (typeof cancel === "function") cancel();
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Elapsed-seconds ticker while connecting.
  useEffect(() => {
    if (!state.connId || state.error) {
      setElapsed(0);
      return;
    }
    const timer = setInterval(() => setElapsed(Math.floor((Date.now() - state.startedAt) / 1000)), 500);
    return () => clearInterval(timer);
  }, [state.connId, state.error, state.startedAt]);

  if (!state.connId) return null;

  const retry = () => {
    const fn = state.retry;
    state.dismiss();
    fn?.();
  };

  const editHost = () => {
    const hostId = state.hostId;
    state.dismiss();
    // Deep-link to the terminal view with this host's editor open. The
    // editor dialog is globally mounted (AppShell); the HostDTO comes from
    // the hosts query cache.
    setView("terminal");
    if (hostId) {
      const host = (queryClient.getQueryData<HostDTO[]>(HOSTS_KEY) ?? []).find((h) => h.id === hostId);
      if (host) openEditor(host);
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50">
      {state.error ? (
        // ── Failure dialog ────────────────────────────────────────────
        (() => {
          const { hint } = classifyConnectError(state.error);
          return (
            <div
              role="alertdialog"
              aria-modal="true"
              className="w-[26rem] max-w-[90vw] rounded-[var(--radius)] border border-border bg-card p-5 shadow-2xl"
            >
              <div className="flex items-start gap-3">
                <CircleAlert className="mt-0.5 h-5 w-5 shrink-0 text-destructive" />
                <div className="min-w-0 flex-1">
                  <h3 className="text-sm font-semibold text-foreground">
                    {t("connect.failedTitle", { name: state.hostName })}
                  </h3>
                  <p className="mt-1.5 text-xs leading-relaxed text-foreground/90">{t(`connect.${hint}`)}</p>
                  <pre className="mt-3 max-h-32 overflow-auto whitespace-pre-wrap break-all rounded-[var(--radius)] border border-border bg-background p-2 font-mono text-[10px] leading-relaxed text-muted-foreground">
                    {state.error}
                  </pre>
                </div>
              </div>
              <div className="mt-4 flex justify-end gap-2">
                <button
                  type="button"
                  onClick={() => state.dismiss()}
                  className="rounded-[var(--radius)] px-3 py-1.5 text-xs text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
                >
                  {t("common.close")}
                </button>
                {state.hostId && (
                  <button
                    type="button"
                    onClick={editHost}
                    className="flex items-center gap-1.5 rounded-[var(--radius)] border border-border px-3 py-1.5 text-xs text-foreground transition-colors hover:bg-accent"
                  >
                    <Pencil className="h-3 w-3" />
                    {t("connect.editHost")}
                  </button>
                )}
                {state.retry && (
                  <button
                    type="button"
                    onClick={retry}
                    className="flex items-center gap-1.5 rounded-[var(--radius)] bg-primary px-3 py-1.5 text-xs font-medium text-primary-foreground transition-colors hover:bg-primary/90"
                  >
                    <RotateCw className="h-3 w-3" />
                    {t("connect.retry")}
                  </button>
                )}
              </div>
            </div>
          );
        })()
      ) : (
        // ── Connecting card ───────────────────────────────────────────
        <div className="w-[22rem] max-w-[90vw] rounded-[var(--radius)] border border-border bg-card p-5 shadow-2xl">
          <div className="flex items-center gap-3">
            <Loader2 className="h-5 w-5 shrink-0 animate-spin text-primary" />
            <div className="min-w-0 flex-1">
              <h3 className="truncate text-sm font-semibold text-foreground">
                {t("connect.connectingTitle", { name: state.hostName })}
              </h3>
              <p className="text-[10px] text-muted-foreground">
                {t("connect.elapsed", { sec: elapsed })}
              </p>
            </div>
          </div>

          <div className="mt-4 flex flex-col gap-2">
            {CONNECT_STAGES.map((s) => {
              const idx = CONNECT_STAGES.indexOf(s);
              const currentIdx = CONNECT_STAGES.indexOf(state.stage);
              const done = idx < currentIdx;
              const active = idx === currentIdx;
              return (
                <div key={s} className="flex items-center gap-2.5 text-xs">
                  <span
                    className={cn(
                      "flex h-4 w-4 shrink-0 items-center justify-center rounded-full border",
                      done
                        ? "border-primary bg-primary/15 text-primary"
                        : active
                          ? "border-primary text-primary"
                          : "border-border text-transparent",
                    )}
                  >
                    {done ? (
                      <Check className="h-2.5 w-2.5" />
                    ) : active ? (
                      <span className="h-1.5 w-1.5 animate-pulse rounded-full bg-primary" />
                    ) : (
                      <span className="h-1.5 w-1.5 rounded-full" />
                    )}
                  </span>
                  <span className={cn(done || active ? "text-foreground" : "text-muted-foreground/60")}>
                    {t(STAGE_LABELS[s])}
                  </span>
                  {active && <Loader2 className="h-3 w-3 animate-spin text-primary/70" />}
                </div>
              );
            })}
          </div>
        </div>
      )}
    </div>
  );
}
