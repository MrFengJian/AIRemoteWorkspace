// SFTP feature API — typed wrappers over the generated SftpService bindings.
//
// Transfers are streaming and backend-driven (FileZilla-style): the UI only
// picks paths via native dialogs and observes events — file bytes never
// cross the JS↔Go bridge, so memory usage is independent of file size.
// Per-transfer events: progress on "sftp:transfer:<id>", terminal state on
// "sftp:transfer:<id>:end" ("" success | "cancelled" | error text).

import { Events } from "@wailsio/runtime";

import {
  SftpService,
  type FileEntryDTO,
} from "@/../bindings/github.com/ai-remote/workspace/internal/interfaces";

export type { FileEntryDTO };

/** Progress payload emitted by the backend during a transfer. */
export interface TransferProgress {
  transferred: number;
  total: number;
}

/** Terminal state of a streaming transfer. */
export type TransferEnd = { ok: true } | { ok: false; cancelled: boolean; error: string };

let nextTransferId = 1;

/** Fresh id for one upload/download (scoped to the app run). */
export function newTransferId(): string {
  return `t${Date.now().toString(36)}${nextTransferId++}`;
}

/** Subscribe to a transfer's progress events; returns an unsubscribe fn. */
export function onTransferProgress(
  id: string,
  cb: (p: TransferProgress) => void,
): () => void {
  const cancel = Events.On(`sftp:transfer:${id}`, (e: unknown) => {
    const data = (e as { data?: unknown }).data as TransferProgress | undefined;
    if (data) cb(data);
  });
  return () => {
    if (typeof cancel === "function") cancel();
  };
}

/**
 * Subscribe to a transfer's terminal event; returns an unsubscribe fn.
 * The promise-style helper below is the convenient way to await it.
 */
export function onTransferEnd(id: string, cb: (end: TransferEnd) => void): () => void {
  const cancel = Events.On(`sftp:transfer:${id}:end`, (e: unknown) => {
    const data = (e as { data?: unknown }).data;
    const msg = typeof data === "string" ? data : "";
    cb(msg === "" ? { ok: true } : { ok: false, cancelled: msg === "cancelled", error: msg });
  });
  return () => {
    if (typeof cancel === "function") cancel();
  };
}

/** Resolve when the transfer finishes (success or failure). */
export function transferDone(id: string): Promise<TransferEnd> {
  return new Promise((resolve) => {
    const cancel = onTransferEnd(id, (end) => {
      cancel();
      resolve(end);
    });
  });
}

export interface SftpApi {
  listDir: (hostID: string, dir: string) => Promise<FileEntryDTO[]>;
  /** Stream remotePath → localPath (native save-dialog path). Directories
   *  transfer as whole trees. */
  startDownload: (hostID: string, remotePath: string, localPath: string, transferID: string) => Promise<void>;
  /** Stream localPath (native open-dialog path) → remotePath. Directories
   *  transfer as whole trees. */
  startUpload: (hostID: string, localPath: string, remotePath: string, transferID: string) => Promise<void>;
  /** Copy a local file/tree to another local path (async, cancellable). */
  startLocalCopy: (srcPath: string, dstPath: string, transferID: string) => Promise<void>;
  /** Copy a remote file/tree to another path on the same host (async). */
  startRemoteCopy: (hostID: string, srcPath: string, dstPath: string, transferID: string) => Promise<void>;
  /** Abort a running transfer by id. */
  cancelTransfer: (transferID: string) => Promise<void>;
  /** Whether a local path exists (download-side same-name conflict check). */
  localExists: (path: string) => Promise<boolean>;
  /** Whether a remote path exists (paste-side same-name conflict check). */
  remoteExists: (hostID: string, path: string) => Promise<boolean>;
  deleteFile: (hostID: string, remotePath: string) => Promise<void>;
  renameFile: (hostID: string, oldPath: string, newPath: string) => Promise<void>;
  mkdir: (hostID: string, remotePath: string) => Promise<void>;
  // ── Local pane (the app machine's own filesystem) ──
  listLocalDir: (dir: string) => Promise<FileEntryDTO[]>;
  /** The local pane's initial directory (the user's home). */
  localDefaultDir: () => Promise<string>;
  localMkdir: (path: string) => Promise<void>;
  localRename: (oldPath: string, newPath: string) => Promise<void>;
  localDelete: (path: string) => Promise<void>;
}

export const sftpApi: SftpApi = {
  listDir: (hostID, dir) => SftpService.ListDir(hostID, dir).then((r) => r ?? []),
  startDownload: (hostID, remotePath, localPath, transferID) =>
    SftpService.StartDownload(hostID, remotePath, localPath, transferID),
  startUpload: (hostID, localPath, remotePath, transferID) =>
    SftpService.StartUpload(hostID, localPath, remotePath, transferID),
  startLocalCopy: (srcPath, dstPath, transferID) =>
    SftpService.StartLocalCopy(srcPath, dstPath, transferID),
  startRemoteCopy: (hostID, srcPath, dstPath, transferID) =>
    SftpService.StartRemoteCopy(hostID, srcPath, dstPath, transferID),
  cancelTransfer: (transferID) => SftpService.CancelTransfer(transferID),
  localExists: (path) => SftpService.LocalExists(path),
  remoteExists: (hostID, path) => SftpService.RemoteExists(hostID, path),
  deleteFile: (hostID, remotePath) => SftpService.DeleteFile(hostID, remotePath),
  renameFile: (hostID, oldPath, newPath) =>
    SftpService.RenameFile(hostID, oldPath, newPath),
  mkdir: (hostID, remotePath) => SftpService.Mkdir(hostID, remotePath),
  listLocalDir: (dir) => SftpService.ListLocalDir(dir).then((r) => r ?? []),
  localDefaultDir: () => SftpService.LocalDefaultDir(),
  localMkdir: (path) => SftpService.LocalMkdir(path),
  localRename: (oldPath, newPath) => SftpService.LocalRename(oldPath, newPath),
  localDelete: (path) => SftpService.LocalDelete(path),
};
