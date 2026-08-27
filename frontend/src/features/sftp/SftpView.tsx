import { useCallback, useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Dialogs } from "@wailsio/runtime";
import {
  Folder,
  File as FileIcon,
  Download,
  Upload,
  RefreshCw,
  Trash2,
  Pencil,
  FolderPlus,
  ArrowLeft,
  ChevronRight,
  Loader2,
  HardDrive,
  Eye,
  EyeOff,
  X,
} from "lucide-react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import {
  sftpApi,
  newTransferId,
  onTransferProgress,
  transferDone,
  type FileEntryDTO,
  type TransferProgress,
} from "@/features/sftp/api";
import { cn } from "@/lib/utils";
import { useConfirm } from "@/lib/useConfirm";
import { toast, errorMessage } from "@/lib/toast";

interface SftpViewProps {
  /** The host this browser operates on — bound to the terminal session's
   *  host (the SFTP panel lives inside the terminal view; there is no
   *  standalone mode anymore). Switching the prop resets the browser. */
  embeddedHostID: string;
  embeddedHostName: string;
}

/** One in-flight upload/download, shown as a progress bar in the status bar. */
interface TransferState extends TransferProgress {
  /** Backend transfer id (cancel hook). */
  id: string;
  /** File name being transferred (display only). */
  name: string;
  /** Which way the bytes flow — picks the status-bar icon. */
  direction: "up" | "down";
}

/** Basename for both POSIX and Windows paths. */
function baseName(p: string): string {
  return p.split(/[\\/]/).pop() || p;
}

/** Split "report.tar.gz" into base + last extension (dotfiles stay whole). */
function splitExt(name: string): { base: string; ext: string } {
  const i = name.lastIndexOf(".");
  if (i <= 0) return { base: name, ext: "" };
  return { base: name.slice(0, i), ext: name.slice(i) };
}

/**
 * First non-conflicting name: name → name(1).ext → name(2).ext … The
 * generator keeps counting so the renamed file can't collide either.
 */
async function suggestName(
  name: string,
  exists: (n: string) => boolean | Promise<boolean>,
): Promise<string> {
  if (!(await exists(name))) return name;
  const { base, ext } = splitExt(name);
  for (let i = 1; i <= 99; i++) {
    const candidate = `${base}(${i})${ext}`;
    if (!(await exists(candidate))) return candidate;
  }
  return `${base}(${Date.now()})${ext}`;
}

/**
 * SFTP file browser, embedded in the terminal view's right panel. Lists a
 * remote directory of the session's host, with download/upload/delete/rename/
 * mkdir. Credentials are resolved by the backend from the OS vault — no
 * password entry here.
 *
 * All browse state lives in the component (not a global store); switching the
 * host prop (terminal tab switch) resets the browser to "/".
 */
export function SftpView({ embeddedHostID, embeddedHostName }: SftpViewProps) {
  const { t } = useTranslation();

  // ── Browse state (component-local; see class comment) ──────────────
  const [hostId, setHostId] = useState<string>(embeddedHostID);
  const [cwd, setCwd] = useState("/");
  const [entries, setEntries] = useState<FileEntryDTO[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [showHidden, setShowHidden] = useState(false);
  const [transfer, setTransfer] = useState<TransferState | null>(null);

  const [pathInput, setPathInput] = useState("/");
  const [renaming, setRenaming] = useState<string | null>(null);
  const [renameValue, setRenameValue] = useState("");
  const { askConfirm, askPrompt, askChoice } = useConfirm();

  // Follow the host prop; switching hosts resets the browser.
  useEffect(() => {
    if (embeddedHostID !== hostId) {
      setHostId(embeddedHostID);
      setCwd("/");
      setEntries([]);
      setError(null);
    }
  }, [embeddedHostID, hostId]);

  // Keep the path input in sync with cwd.
  useEffect(() => setPathInput(cwd), [cwd]);

  const refresh = useCallback(
    async (dir: string) => {
      if (!hostId) return;
      setLoading(true);
      setError(null);
      try {
        const list = await sftpApi.listDir(hostId, dir);
        setEntries(list);
      } catch (e) {
        setError(errorMessage(e));
        setEntries([]);
      } finally {
        setLoading(false);
      }
    },
    [hostId],
  );

  // Auto-refresh when host or cwd changes.
  useEffect(() => {
    if (hostId) refresh(cwd);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [hostId, cwd]);

  const navigate = (dir: string) => {
    setCwd(dir);
    setError(null);
  };

  const goUp = () => {
    const parent = cwd.replace(/\/[^/]+\/?$/, "") || "/";
    navigate(parent === "" ? "/" : parent);
  };

  const openEntry = (entry: FileEntryDTO) => {
    if (entry.isDir) {
      navigate(cwd.replace(/\/$/, "") + "/" + entry.name);
    } else {
      handleDownload(entry);
    }
  };

  const busy = transfer !== null;

  /** Runs one streaming transfer: registers progress, starts the backend
   *  job, and resolves when its terminal event arrives. Cancellation is a
   *  quiet notice, not an error. */
  const runStream = async (
    name: string,
    direction: "up" | "down",
    start: (transferID: string) => Promise<void>,
  ): Promise<void> => {
    const id = newTransferId();
    const unsubscribe = onTransferProgress(id, (p) =>
      setTransfer({ id, name, direction, ...p }),
    );
    setTransfer({ id, name, direction, transferred: 0, total: 0 });
    try {
      await start(id);
      const end = await transferDone(id);
      if (!end.ok) {
        if (end.cancelled) {
          toast.info(t("sftp.cancelled", { name }));
          return;
        }
        throw new Error(end.error);
      }
    } finally {
      unsubscribe();
      setTransfer(null);
    }
  };

  const handleDownload = async (entry: FileEntryDTO) => {
    if (!hostId || busy) return;
    const fullPath = cwd.replace(/\/$/, "") + "/" + entry.name;
    // Native save dialog: a real filesystem path the backend streams to —
    // file bytes never cross the JS↔Go bridge, so size doesn't matter.
    let localPath = await Dialogs.SaveFile({ Filename: entry.name });
    if (!localPath) return; // dialog dismissed
    // Same-name conflict on the local side: overwrite / auto-rename / abort.
    if (await sftpApi.localExists(localPath)) {
      const choice = await askChoice({
        title: t("sftp.conflictTitle"),
        message: t("sftp.conflictMessage", { name: baseName(localPath) }),
        confirmLabel: t("sftp.conflictOverwrite"),
        altLabel: t("sftp.conflictRename"),
        danger: true,
      });
      if (choice === false) return;
      if (choice === "alt") {
        const idx = Math.max(localPath.lastIndexOf("/"), localPath.lastIndexOf("\\"));
        const dir = idx >= 0 ? localPath.slice(0, idx + 1) : "";
        const renamed = await suggestName(baseName(localPath), (n) =>
          sftpApi.localExists(dir + n),
        );
        localPath = dir + renamed;
        toast.info(t("sftp.renamedTo", { name: renamed }));
      }
    }
    try {
      await runStream(entry.name, "down", (transferID) =>
        sftpApi.startDownload(hostId, fullPath, localPath, transferID),
      );
      toast.success(t("sftp.downloadDone", { name: entry.name }));
    } catch (e) {
      toast.error(`${t("sftp.downloadFailed", { name: entry.name })}: ${errorMessage(e)}`);
    }
  };

  const handleUpload = async () => {
    if (!hostId || busy) return;
    // Native open dialog returns real paths → backend-side streaming.
    const localPath = await Dialogs.OpenFile({ CanChooseFiles: true });
    if (!localPath) return; // dialog dismissed
    let name = baseName(localPath);
    // Same-name conflict in the remote directory (checked against the
    // current listing): overwrite / auto-rename / abort.
    const remoteHas = (n: string) => entries.some((e) => e.name === n);
    if (remoteHas(name)) {
      const choice = await askChoice({
        title: t("sftp.conflictTitle"),
        message: t("sftp.conflictMessage", { name }),
        confirmLabel: t("sftp.conflictOverwrite"),
        altLabel: t("sftp.conflictRename"),
        danger: true,
      });
      if (choice === false) return;
      if (choice === "alt") {
        name = await suggestName(name, remoteHas);
        toast.info(t("sftp.renamedTo", { name }));
      }
    }
    const fullPath = cwd.replace(/\/$/, "") + "/" + name;
    try {
      await runStream(name, "up", (transferID) =>
        sftpApi.startUpload(hostId, localPath, fullPath, transferID),
      );
      toast.success(t("sftp.uploadDone", { name }));
      await refresh(cwd);
    } catch (e) {
      toast.error(`${t("sftp.uploadFailed", { name })}: ${errorMessage(e)}`);
    }
  };

  const handleDelete = async (entry: FileEntryDTO) => {
    if (!hostId) return;
    const ok = await askConfirm({
      title: t("sftp.deleteTitle"),
      message: t("sftp.deleteConfirm", { name: entry.name }),
      danger: true,
      confirmLabel: t("common.delete"),
    });
    if (!ok) return;
    const fullPath = cwd.replace(/\/$/, "") + "/" + entry.name;
    try {
      await sftpApi.deleteFile(hostId, fullPath);
      await refresh(cwd);
    } catch (e) {
      toast.error(`${t("sftp.deleteFailed", { name: entry.name })}: ${errorMessage(e)}`);
    }
  };

  const startRename = (entry: FileEntryDTO) => {
    setRenaming(entry.name);
    setRenameValue(entry.name);
  };

  const commitRename = async (oldName: string) => {
    if (!hostId || !renameValue || renameValue === oldName) {
      setRenaming(null);
      return;
    }
    const oldPath = cwd.replace(/\/$/, "") + "/" + oldName;
    const newPath = cwd.replace(/\/$/, "") + "/" + renameValue;
    try {
      await sftpApi.renameFile(hostId, oldPath, newPath);
      await refresh(cwd);
    } catch (e) {
      toast.error(`${t("sftp.renameFailed", { name: oldName })}: ${errorMessage(e)}`);
    }
    setRenaming(null);
  };

  const handleMkdir = async () => {
    if (!hostId) return;
    const name = await askPrompt({
      title: t("sftp.newFolderTitle"),
      placeholder: t("sftp.newFolderPrompt"),
      confirmLabel: t("sftp.newFolder"),
    });
    if (!name?.trim()) return;
    const fullPath = cwd.replace(/\/$/, "") + "/" + name.trim();
    try {
      await sftpApi.mkdir(hostId, fullPath);
      await refresh(cwd);
    } catch (e) {
      toast.error(`${t("sftp.newFolder")} : ${errorMessage(e)}`);
    }
  };

  const submitPath = () => {
    let p = pathInput.trim() || "/";
    if (!p.startsWith("/")) p = "/" + p;
    navigate(p);
  };

  // Dotfiles ("." and ".." never come through; entries like ".bashrc") are
  // filtered client-side so the toggle is instant — no refetch needed.
  const visibleEntries = showHidden ? entries : entries.filter((e) => !e.name.startsWith("."));

  return (
    <div className="flex h-full flex-col">
      {/* Toolbar */}
      <div className="flex flex-wrap items-center gap-2 border-b border-border bg-card px-3 py-2">
        {/* The host this browser is bound to (the terminal session's host). */}
        <div className="flex min-w-0 items-center gap-1.5 text-xs text-muted-foreground">
          <HardDrive className="h-3.5 w-3.5 shrink-0 text-primary" />
          <span className="max-w-[9rem] truncate" title={embeddedHostName}>
            {embeddedHostName}
          </span>
        </div>

        <Button variant="ghost" size="icon" className="h-8 w-8" onClick={goUp} title={t("sftp.up")}>
          <ArrowLeft className="h-4 w-4" />
        </Button>

        {/* Path input */}
        <Input
          className="h-8 flex-1 font-mono text-xs"
          value={pathInput}
          onChange={(e) => setPathInput(e.target.value)}
          onKeyDown={(e) => e.key === "Enter" && submitPath()}
          placeholder="/"
        />
        <Button
          variant="ghost"
          size="icon"
          className="h-8 w-8"
          onClick={() => refresh(cwd)}
          disabled={busy}
          title={t("common.refresh")}
        >
          <RefreshCw className={cn("h-4 w-4", loading && "animate-spin")} />
        </Button>
        <Button
          variant="ghost"
          size="icon"
          className={cn("h-8 w-8", showHidden && "bg-accent text-primary")}
          onClick={() => setShowHidden(!showHidden)}
          title={showHidden ? t("sftp.hideHidden") : t("sftp.showHidden")}
        >
          {showHidden ? <Eye className="h-4 w-4" /> : <EyeOff className="h-4 w-4" />}
        </Button>

        <div className="mx-1 h-5 w-px bg-border" />

        <Button
          variant="ghost"
          size="icon"
          className="h-8 w-8"
          onClick={handleMkdir}
          disabled={busy}
          title={t("sftp.newFolder")}
        >
          <FolderPlus className="h-4 w-4" />
        </Button>
        <Button
          variant="ghost"
          size="icon"
          className="h-8 w-8"
          onClick={() => void handleUpload()}
          disabled={busy}
          title={t("sftp.upload")}
        >
          {busy ? <Loader2 className="h-4 w-4 animate-spin" /> : <Upload className="h-4 w-4" />}
        </Button>
      </div>

      {/* Breadcrumb */}
      <div className="flex items-center gap-0.5 border-b border-border px-3 py-1 text-xs text-muted-foreground">
        {splitPath(cwd).map((part, i, arr) => {
          const path = arr.slice(0, i + 1).join("/") || "/";
          return (
            <span key={i} className="flex items-center">
              {i > 0 && <ChevronRight className="mx-0.5 h-3 w-3" />}
              <button
                type="button"
                className="rounded px-1 hover:bg-accent hover:text-foreground"
                onClick={() => navigate(path)}
              >
                {i === 0 ? t("sftp.root") : part}
              </button>
            </span>
          );
        })}
      </div>

      {/* Error banner */}
      {error && (
        <div className="border-b border-destructive/30 bg-destructive/10 px-3 py-1.5 text-xs text-destructive">
          {error}
        </div>
      )}

      {/* File list */}
      <div className="min-h-0 flex-1 overflow-auto">
        {loading && visibleEntries.length === 0 ? (
          <div className="flex items-center justify-center py-12 text-sm text-muted-foreground">
            <Loader2 className="mr-2 h-4 w-4 animate-spin" /> {t("common.loading")}
          </div>
        ) : visibleEntries.length === 0 ? (
          <div className="flex items-center justify-center py-12 text-sm text-muted-foreground">
            {t("sftp.empty")}
          </div>
        ) : (
          <table className="w-full text-sm">
            <thead>
              <tr className="sticky top-0 border-b border-border bg-card text-left text-xs text-muted-foreground">
                <th className="w-8 py-1.5 pl-3" aria-label={t("sftp.colName")} />
                <th className="py-1.5 pr-2 font-medium">{t("sftp.colName")}</th>
                <th className="hidden w-28 py-1.5 pr-3 text-right font-medium sm:table-cell">
                  {t("sftp.colSize")}
                </th>
                <th className="hidden w-40 py-1.5 pr-3 font-medium md:table-cell">
                  {t("sftp.colModified")}
                </th>
                <th className="w-24 py-1.5 pr-2" aria-label={t("common.edit")} />
              </tr>
            </thead>
            <tbody>
              {visibleEntries.map((entry) => (
                <tr
                  key={entry.name}
                  className="group border-b border-border/50 hover:bg-accent/30"
                  onDoubleClick={() => openEntry(entry)}
                >
                  <td className="w-8 py-1.5 pl-3">
                    {entry.isDir ? (
                      <Folder className="h-4 w-4 text-primary" />
                    ) : (
                      <FileIcon className="h-4 w-4 text-muted-foreground" />
                    )}
                  </td>
                  <td className="py-1.5 pr-2">
                    {renaming === entry.name ? (
                      <Input
                        autoFocus
                        className="h-6 py-0 text-xs"
                        value={renameValue}
                        onChange={(e) => setRenameValue(e.target.value)}
                        onBlur={() => commitRename(entry.name)}
                        onKeyDown={(e) => {
                          if (e.key === "Enter") commitRename(entry.name);
                          if (e.key === "Escape") setRenaming(null);
                        }}
                      />
                    ) : (
                      <button
                        type="button"
                        className="truncate text-left text-foreground"
                        onClick={() => openEntry(entry)}
                      >
                        {entry.name}
                      </button>
                    )}
                  </td>
                  <td className="hidden w-28 py-1.5 pr-3 text-right font-mono text-xs text-muted-foreground sm:table-cell">
                    {entry.isDir ? "—" : formatSize(entry.size)}
                  </td>
                  <td className="hidden w-40 py-1.5 pr-3 text-xs text-muted-foreground md:table-cell">
                    {entry.modTime ? new Date(entry.modTime).toLocaleString() : ""}
                  </td>
                  <td className="w-24 py-1.5 pr-2 text-right">
                    <div className="flex justify-end gap-0.5 opacity-0 transition-opacity focus-within:opacity-100 group-hover:opacity-100">
                      {!entry.isDir && (
                        <button
                          type="button"
                          className="rounded p-1 hover:bg-accent"
                          title={t("sftp.download")}
                          onClick={(e) => {
                            e.stopPropagation();
                            handleDownload(entry);
                          }}
                        >
                          <Download className="h-3.5 w-3.5" />
                        </button>
                      )}
                      <button
                        type="button"
                        className="rounded p-1 hover:bg-accent"
                        title={t("sftp.rename")}
                        onClick={(e) => {
                          e.stopPropagation();
                          startRename(entry);
                        }}
                      >
                        <Pencil className="h-3.5 w-3.5" />
                      </button>
                      <button
                        type="button"
                        className="rounded p-1 text-destructive hover:bg-accent"
                        title={t("common.delete")}
                        onClick={(e) => {
                          e.stopPropagation();
                          handleDelete(entry);
                        }}
                      >
                        <Trash2 className="h-3.5 w-3.5" />
                      </button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      {/* Status bar: transfer progress or directory summary */}
      {transfer ? (
        <div className="flex items-center gap-2 border-t border-border bg-card px-3 py-1.5 text-xs text-muted-foreground">
          {transfer.direction === "up" ? (
            <Upload className="h-3 w-3 shrink-0" aria-hidden />
          ) : (
            <Download className="h-3 w-3 shrink-0" aria-hidden />
          )}
          <span className="max-w-[12rem] shrink-0 truncate">
            {transfer.name}
          </span>
          <div className="h-1.5 min-w-0 flex-1 overflow-hidden rounded-full bg-secondary">
            <div
              className="h-full rounded-full bg-primary transition-[width] duration-150"
              style={{
                width: transfer.total > 0
                  ? `${Math.min(100, (transfer.transferred / transfer.total) * 100)}%`
                  : "100%",
              }}
            />
          </div>
          {transfer.total > 0 && (
            <span className="w-9 shrink-0 text-right font-mono tabular-nums text-foreground">
              {Math.min(100, Math.floor((transfer.transferred / transfer.total) * 100))}%
            </span>
          )}
          <span className="shrink-0 font-mono">
            {transfer.total > 0
              ? `${formatSize(transfer.transferred)} / ${formatSize(transfer.total)}`
              : formatSize(transfer.transferred)}
          </span>
          <button
            type="button"
            onClick={() => void sftpApi.cancelTransfer(transfer.id)}
            title={t("sftp.cancelTransfer")}
            className="rounded p-0.5 text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
          >
            <X className="h-3 w-3" />
          </button>
        </div>
      ) : (
        <div className="flex items-center justify-between border-t border-border bg-card px-3 py-1 text-xs text-muted-foreground">
          <span>
            {embeddedHostName} · {visibleEntries.length} {t("sftp.items")}
            {!showHidden && entries.length > visibleEntries.length && (
              <span className="ml-1">{t("sftp.hiddenCount", { count: entries.length - visibleEntries.length })}</span>
            )}
          </span>
          {loading && <Badge variant="secondary">{t("sftp.refreshing")}</Badge>}
        </div>
      )}
    </div>
  );
}

function splitPath(p: string): string[] {
  return p.split("/").filter(Boolean);
}

function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  if (bytes < 1024 * 1024 * 1024) return `${(bytes / 1024 / 1024).toFixed(1)} MB`;
  return `${(bytes / 1024 / 1024 / 1024).toFixed(1)} GB`;
}
