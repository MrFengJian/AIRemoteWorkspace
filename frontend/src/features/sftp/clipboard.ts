// App-internal file clipboard for the SFTP panel's copy/paste. It records
// one copied entry (file or folder) plus which side it came from, so a paste
// can resolve its semantics: same side → plain copy, local→remote → upload,
// remote→local → download. Pasting is only offered while this holds an
// entry — the OS text clipboard is never consulted for file transfers.

export type Side = "local" | "remote";

/** One copied entry waiting to be pasted. */
export interface FileClipboard {
  side: Side;
  /** Source host id (empty for the local side). */
  hostId: string;
  name: string;
  /** Full source path (POSIX for remote, native for local). */
  path: string;
  isDir: boolean;
}

let current: FileClipboard | null = null;
const listeners = new Set<() => void>();

export function getFileClipboard(): FileClipboard | null {
  return current;
}

export function setFileClipboard(next: FileClipboard | null): void {
  current = next;
  listeners.forEach((l) => l());
}

/** Subscribe to clipboard changes; returns an unsubscribe function. */
export function onClipboardChange(l: () => void): () => void {
  listeners.add(l);
  return () => {
    listeners.delete(l);
  };
}
