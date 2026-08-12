// SFTP feature API — typed wrappers over the generated SftpService bindings.
//
// DownloadFile/UploadFile carry file bytes as strings (Wails serialises
// []byte ↔ string). Convert in the helpers so callers stay typed.

import {
  SftpService,
  type FileEntryDTO,
} from "@/../bindings/github.com/ai-remote/workspace/internal/interfaces";

export type { FileEntryDTO };

export interface SftpApi {
  listDir: (hostID: string, dir: string) => Promise<FileEntryDTO[]>;
  downloadFile: (hostID: string, remotePath: string) => Promise<Uint8Array>;
  uploadFile: (hostID: string, remotePath: string, data: Uint8Array) => Promise<void>;
  deleteFile: (hostID: string, remotePath: string) => Promise<void>;
  renameFile: (hostID: string, oldPath: string, newPath: string) => Promise<void>;
  mkdir: (hostID: string, remotePath: string) => Promise<void>;
}

export const sftpApi: SftpApi = {
  listDir: (hostID, dir) => SftpService.ListDir(hostID, dir).then((r) => r ?? []),
  downloadFile: (hostID, remotePath) =>
    // Backend returns the file bytes as a string; encode to bytes for the Blob.
    SftpService.DownloadFile(hostID, remotePath).then((s) =>
      new TextEncoder().encode(s ?? ""),
    ),
  uploadFile: (hostID, remotePath, data) =>
    // Backend expects a string; decode bytes first.
    SftpService.UploadFile(hostID, remotePath, new TextDecoder().decode(data)),
  deleteFile: (hostID, remotePath) => SftpService.DeleteFile(hostID, remotePath),
  renameFile: (hostID, oldPath, newPath) =>
    SftpService.RenameFile(hostID, oldPath, newPath),
  mkdir: (hostID, remotePath) => SftpService.Mkdir(hostID, remotePath),
};
