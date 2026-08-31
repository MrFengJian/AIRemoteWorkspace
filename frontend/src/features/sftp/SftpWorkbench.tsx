import { useCallback, useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Dialogs } from "@wailsio/runtime";
import {
  Copy,
  Download,
  HardDrive,
  Upload,
  X,
  Cloud,
} from "lucide-react";

import {
  sftpApi,
  newTransferId,
  onTransferProgress,
  transferDone,
  type FileEntryDTO,
  type TransferProgress,
} from "@/features/sftp/api";
import { BrowserPane } from "@/features/sftp/BrowserPane";
import { useBrowser } from "@/features/sftp/useBrowser";
import { getFileClipboard, onClipboardChange, setFileClipboard } from "@/features/sftp/clipboard";
import type { Side } from "@/features/sftp/clipboard";
import { joinPath } from "@/features/sftp/paths";
import { baseName, formatSize, suggestName } from "@/features/sftp/format";
import { useConfirm } from "@/lib/useConfirm";
import { toast, errorMessage } from "@/lib/toast";

interface SftpWorkbenchProps {
  /** The host whose remote filesystem the right pane browses. */
  hostID: string;
}

/** One in-flight transfer (upload/download/copy), shown in the status bar. */
interface TransferState extends TransferProgress {
  /** Backend transfer id (cancel hook). */
  id: string;
  /** Name being transferred (display only). */
  name: string;
  /** Which way the bytes flow — picks the status-bar icon. */
  direction: "up" | "down" | "copy";
}

/**
 * Dual-pane SFTP workbench, FileZilla-style: local pane on the left, remote
 * pane on the right. Copy/paste between (or within) the panes resolves by
 * side: same side → plain copy, local→remote → upload, remote→local →
 * download. Used by the standalone SFTP window (opened per host from the
 * hosts sidebar); the embedded terminal panel keeps its remote-only view.
 */
export function SftpWorkbench({ hostID }: SftpWorkbenchProps) {
  const { t } = useTranslation();
  const { askChoice } = useConfirm();

  const [transfer, setTransfer] = useState<TransferState | null>(null);

  // ── Clipboard (app-internal; paste menus subscribe through this state) ──
  const [clipboard, setClipboardState] = useState(getFileClipboard());
  useEffect(() => onClipboardChange(() => setClipboardState(getFileClipboard())), []);

  // ── Pane states: local left, remote right ───────────────────────────
  const listLocal = useCallback((dir: string) => sftpApi.listLocalDir(dir), []);
  const listRemote = useCallback(
    (dir: string) => sftpApi.listDir(hostID, dir),
    [hostID],
  );
  const local = useBrowser("local", listLocal, () => sftpApi.localDefaultDir());
  const remote = useBrowser("remote", listRemote, async () => "/");

  const busy = transfer !== null;

  /** Paste enablement: the clipboard holds an entry, no transfer is running,
   *  and a remote source belongs to this workbench's host (cross-host
   *  transfers are not supported). */
  const pasteEnabled =
    clipboard !== null &&
    !busy &&
    !(clipboard.side === "remote" && (!clipboard.hostId || clipboard.hostId !== hostID));

  /** Runs one streaming transfer: registers progress, starts the backend
   *  job, and resolves when its terminal event arrives. Cancellation is a
   *  quiet notice, not an error. */
  const runStream = async (
    name: string,
    direction: TransferState["direction"],
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

  const handleCopy = (side: Side, entry: FileEntryDTO) => {
    if (side === "remote" && !hostID) return;
    const cwd = side === "local" ? local.cwd : remote.cwd;
    setFileClipboard({
      side,
      hostId: side === "remote" ? hostID : "",
      name: entry.name,
      path: joinPath(side, cwd, entry.name),
      isDir: entry.isDir,
    });
    toast.info(t("sftp.copiedReady", { name: entry.name }));
  };

  /** Copy an entry to dstDir on dstSide with same-name conflict resolution
   *  (overwrite / auto-rename / abort), routing by direction:
   *  copy = same-side, up = local→remote, down = remote→local. */
  const transferTo = async (
    direction: TransferState["direction"],
    name: string,
    srcPath: string,
    dstSide: Side,
    dstDir: string,
  ): Promise<void> => {
    const exists = (n: string) => {
      const p = joinPath(dstSide, dstDir, n);
      return dstSide === "local"
        ? sftpApi.localExists(p)
        : sftpApi.remoteExists(hostID, p);
    };
    let finalName = name;
    if (await exists(finalName)) {
      const choice = await askChoice({
        title: t("sftp.conflictTitle"),
        message: t("sftp.conflictMessage", { name: finalName }),
        confirmLabel: t("sftp.conflictOverwrite"),
        altLabel: t("sftp.conflictRename"),
        danger: true,
      });
      if (choice === false) return;
      if (choice === "alt") {
        finalName = await suggestName(finalName, exists);
        toast.info(t("sftp.renamedTo", { name: finalName }));
      }
    }
    const dst = joinPath(dstSide, dstDir, finalName);
    if (direction === "up") {
      await runStream(name, "up", (id) =>
        sftpApi.startUpload(hostID, srcPath, dst, id),
      );
    } else if (direction === "down") {
      await runStream(name, "down", (id) =>
        sftpApi.startDownload(hostID, srcPath, dst, id),
      );
    } else if (dstSide === "local") {
      await runStream(name, "copy", (id) => sftpApi.startLocalCopy(srcPath, dst, id));
    } else {
      await runStream(name, "copy", (id) =>
        sftpApi.startRemoteCopy(hostID, srcPath, dst, id),
      );
    }
    (dstSide === "local" ? local : remote).refresh();
  };

  /** Paste the clipboard entry into a pane directory. Same side → plain
   *  file copy; local→remote → upload; remote→local → download. */
  const handlePaste = async (targetSide: Side, targetDir: string) => {
    if (!clipboard || busy) return;
    if (clipboard.side === "remote" && clipboard.hostId !== hostID) return;
    const direction =
      clipboard.side === targetSide
        ? "copy"
        : clipboard.side === "local"
          ? "up"
          : "down";
    try {
      await transferTo(direction, clipboard.name, clipboard.path, targetSide, targetDir);
      if (direction === "copy") {
        toast.success(t("sftp.copyDone", { name: clipboard.name }));
      } else if (direction === "up") {
        toast.success(t("sftp.uploadDone", { name: clipboard.name }));
      } else {
        toast.success(t("sftp.downloadDone", { name: clipboard.name }));
      }
    } catch (e) {
      toast.error(`${t("sftp.pasteFailed", { name: clipboard.name })}: ${errorMessage(e)}`);
    }
  };

  // ── Pane row actions: quick cross-pane transfer of one entry ─────────

  /** Local row → upload into the remote pane's current directory. */
  const uploadEntry = async (entry: FileEntryDTO) => {
    if (busy || !hostID) return;
    try {
      await transferTo("up", entry.name, joinPath("local", local.cwd, entry.name), "remote", remote.cwd);
      toast.success(t("sftp.uploadDone", { name: entry.name }));
    } catch (e) {
      toast.error(`${t("sftp.uploadFailed", { name: entry.name })}: ${errorMessage(e)}`);
    }
  };

  /** Remote row → download into the local pane's current directory. */
  const downloadEntry = async (entry: FileEntryDTO) => {
    if (busy || !hostID) return;
    try {
      await transferTo("down", entry.name, joinPath("remote", remote.cwd, entry.name), "local", local.cwd);
      toast.success(t("sftp.downloadDone", { name: entry.name }));
    } catch (e) {
      toast.error(`${t("sftp.downloadFailed", { name: entry.name })}: ${errorMessage(e)}`);
    }
  };

  // ── Toolbar upload (native dialog → remote pane cwd) ─────────────────

  const handleUpload = async () => {
    if (!hostID || busy) return;
    const localPath = await Dialogs.OpenFile({ CanChooseFiles: true });
    if (!localPath) return; // dialog dismissed
    try {
      await transferTo("up", baseName(localPath), localPath, "remote", remote.cwd);
      toast.success(t("sftp.uploadDone", { name: baseName(localPath) }));
    } catch (e) {
      toast.error(`${t("sftp.uploadFailed", { name: baseName(localPath) })}: ${errorMessage(e)}`);
    }
  };

  // ── Per-pane mkdir / rename / delete (API + toast + refresh) ─────────

  const makeMkdir = (side: Side) => async (name: string) => {
    try {
      if (side === "local") {
        await sftpApi.localMkdir(joinPath("local", local.cwd, name));
      } else {
        await sftpApi.mkdir(hostID, joinPath("remote", remote.cwd, name));
      }
      (side === "local" ? local : remote).refresh();
    } catch (e) {
      toast.error(t("sftp.mkdirFailed", { err: errorMessage(e) }));
    }
  };

  const makeRename = (side: Side) => async (oldName: string, newName: string) => {
    try {
      if (side === "local") {
        await sftpApi.localRename(
          joinPath("local", local.cwd, oldName),
          joinPath("local", local.cwd, newName),
        );
      } else {
        await sftpApi.renameFile(
          hostID,
          joinPath("remote", remote.cwd, oldName),
          joinPath("remote", remote.cwd, newName),
        );
      }
      (side === "local" ? local : remote).refresh();
    } catch (e) {
      toast.error(`${t("sftp.renameFailed", { name: oldName })}: ${errorMessage(e)}`);
    }
  };

  const makeDelete = (side: Side) => async (entry: FileEntryDTO) => {
    try {
      if (side === "local") {
        await sftpApi.localDelete(joinPath("local", local.cwd, entry.name));
      } else {
        await sftpApi.deleteFile(hostID, joinPath("remote", remote.cwd, entry.name));
      }
      (side === "local" ? local : remote).refresh();
    } catch (e) {
      toast.error(`${t("sftp.deleteFailed", { name: entry.name })}: ${errorMessage(e)}`);
    }
  };

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      {/* Workbench: local pane (left) | remote pane (right) */}
      <div className="flex min-h-0 flex-1">
        <BrowserPane
          browser={local}
          label={t("sftp.localLabel")}
          labelIcon={HardDrive}
          busy={busy}
          pasteEnabled={pasteEnabled}
          onMkdir={makeMkdir("local")}
          onCopy={(entry) => handleCopy("local", entry)}
          onPaste={(dir) => void handlePaste("local", dir)}
          onRename={makeRename("local")}
          onDelete={makeDelete("local")}
          onTransfer={(entry) => void uploadEntry(entry)}
          transferIcon={Upload}
          transferLabel={t("sftp.upload")}
        />

        <div className="w-px shrink-0 bg-border" aria-hidden />

        <BrowserPane
          browser={remote}
          label={t("sftp.remoteLabel")}
          labelIcon={Cloud}
          busy={busy}
          pasteEnabled={pasteEnabled}
          onUpload={() => void handleUpload()}
          onMkdir={makeMkdir("remote")}
          onCopy={(entry) => handleCopy("remote", entry)}
          onPaste={(dir) => void handlePaste("remote", dir)}
          onRename={makeRename("remote")}
          onDelete={makeDelete("remote")}
          onTransfer={(entry) => void downloadEntry(entry)}
          transferIcon={Download}
          transferLabel={t("sftp.download")}
          onOpenFile={(entry) => void downloadEntry(entry)}
        />
      </div>

      {/* Status bar: transfer progress or idle hint */}
      {transfer ? (
        <div className="flex items-center gap-2 border-t border-border bg-card px-3 py-1.5 text-xs text-muted-foreground">
          {transfer.direction === "up" ? (
            <Upload className="h-3 w-3 shrink-0" aria-hidden />
          ) : transfer.direction === "down" ? (
            <Download className="h-3 w-3 shrink-0" aria-hidden />
          ) : (
            <Copy className="h-3 w-3 shrink-0" aria-hidden />
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
        <div className="flex items-center justify-center border-t border-border bg-card px-3 py-1 text-[11px] text-muted-foreground">
          <span>{t("sftp.dualPaneHint")}</span>
        </div>
      )}
    </div>
  );
}
