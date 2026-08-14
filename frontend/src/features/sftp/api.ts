// SFTP feature API — typed wrappers over the generated SftpService bindings.
//
// DownloadFile/UploadFile carry file bytes as strings (Wails serialises
// []byte ↔ string). Both take a caller-generated transfer id; the backend
// emits progress events on "sftp:transfer:<id>" while the transfer runs.

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

export interface SftpApi {
  listDir: (hostID: string, dir: string) => Promise<FileEntryDTO[]>;
  downloadFile: (hostID: string, remotePath: string, transferID: string) => Promise<Uint8Array>;
  uploadFile: (hostID: string, remotePath: string, data: Uint8Array, transferID: string) => Promise<void>;
  deleteFile: (hostID: string, remotePath: string) => Promise<void>;
  renameFile: (hostID: string, oldPath: string, newPath: string) => Promise<void>;
  mkdir: (hostID: string, remotePath: string) => Promise<void>;
}

export const sftpApi: SftpApi = {
  listDir: (hostID, dir) => SftpService.ListDir(hostID, dir).then((r) => r ?? []),
  downloadFile: (hostID, remotePath, transferID) =>
    // Backend returns the file bytes as a string; encode to bytes for the Blob.
    SftpService.DownloadFile(hostID, remotePath, transferID).then((s) =>
      new TextEncoder().encode(s ?? ""),
    ),
  uploadFile: (hostID, remotePath, data, transferID) =>
    // Backend expects a string; decode bytes first.
    SftpService.UploadFile(hostID, remotePath, new TextDecoder().decode(data), transferID),
  deleteFile: (hostID, remotePath) => SftpService.DeleteFile(hostID, remotePath),
  renameFile: (hostID, oldPath, newPath) =>
    SftpService.RenameFile(hostID, oldPath, newPath),
  mkdir: (hostID, remotePath) => SftpService.Mkdir(hostID, remotePath),
};
