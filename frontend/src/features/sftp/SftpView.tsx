import { useCallback, useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
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
} from "lucide-react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import { sftpApi, type FileEntryDTO } from "@/features/sftp/api";
import { useSftpStore } from "@/features/sftp/store";
import { useHosts } from "@/features/hosts/hooks";
import { cn } from "@/lib/utils";

/**
 * SFTP file browser. Lists a remote directory per host, with download/upload/
 * delete/rename/mkdir. Credentials are resolved by the backend from the OS
 * vault (Phase 5) — no password entry here.
 *
 * Layout:
 *   ┌ toolbar: host select | path bar | actions ┐
 *   ├ file list (dirs first, then name)         ┤
 *   └ status / error line                       ┘
 */
export function SftpView() {
  const { t } = useTranslation();
  const { data: hosts } = useHosts();
  const { hostId, hostName, cwd, entries, loading, error, setHost, setCwd, setEntries, setLoading, setError } =
    useSftpStore();
  const [pathInput, setPathInput] = useState("/");
  const [renaming, setRenaming] = useState<string | null>(null);
  const [renameValue, setRenameValue] = useState("");
  const fileInputRef = useRef<HTMLInputElement | null>(null);

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
        setError(e instanceof Error ? e.message : String(e));
        setEntries([]);
      }
    },
    [hostId, setEntries, setError, setLoading],
  );

  // Auto-refresh when host or cwd changes.
  useEffect(() => {
    if (hostId) refresh(cwd);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [hostId, cwd]);

  const navigate = (dir: string) => {
    setCwd(dir);
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

  const handleDownload = async (entry: FileEntryDTO) => {
    if (!hostId) return;
    const fullPath = cwd.replace(/\/$/, "") + "/" + entry.name;
    try {
      const data = await sftpApi.downloadFile(hostId, fullPath);
      // Trigger a browser download via a Blob URL.
      const blob = new Blob([data.buffer as ArrayBuffer]);
      const url = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = url;
      a.download = entry.name;
      a.click();
      URL.revokeObjectURL(url);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
  };

  const handleUpload = async (file: File) => {
    if (!hostId) return;
    const fullPath = cwd.replace(/\/$/, "") + "/" + file.name;
    try {
      const buf = new Uint8Array(await file.arrayBuffer());
      await sftpApi.uploadFile(hostId, fullPath, buf);
      await refresh(cwd);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
  };

  const handleDelete = async (entry: FileEntryDTO) => {
    if (!hostId) return;
    if (!confirm(t("sftp.deleteConfirm", { name: entry.name }))) return;
    const fullPath = cwd.replace(/\/$/, "") + "/" + entry.name;
    try {
      await sftpApi.deleteFile(hostId, fullPath);
      await refresh(cwd);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
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
      setError(e instanceof Error ? e.message : String(e));
    }
    setRenaming(null);
  };

  const handleMkdir = async () => {
    if (!hostId) return;
    const name = prompt(t("sftp.newFolderPrompt"));
    if (!name) return;
    const fullPath = cwd.replace(/\/$/, "") + "/" + name;
    try {
      await sftpApi.mkdir(hostId, fullPath);
      await refresh(cwd);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
  };

  const submitPath = () => {
    let p = pathInput.trim() || "/";
    if (!p.startsWith("/")) p = "/" + p;
    navigate(p);
  };

  // No host selected yet.
  if (!hostId) {
    return (
      <div className="mx-auto max-w-3xl p-6">
        <h1 className="text-xl font-semibold tracking-tight">{t("sftp.title")}</h1>
        <p className="mt-0.5 text-sm text-muted-foreground">
          Browse and manage remote files over SFTP.
        </p>
        <div className="mt-6 rounded-[var(--radius)] border border-dashed border-border p-8">
          <p className="mb-3 text-sm text-muted-foreground">{t("sftp.selectHost")}</p>
          <HostSelector hosts={hosts ?? []} onSelect={(id, name) => setHost(id, name)} />
          {(hosts ?? []).length === 0 && (
            <p className="mt-3 text-xs text-muted-foreground">
              {t("sftp.noHosts")}
            </p>
          )}
        </div>
      </div>
    );
  }

  return (
    <div className="flex h-full flex-col">
      {/* Toolbar */}
      <div className="flex flex-wrap items-center gap-2 border-b border-border bg-card px-3 py-2">
        <HardDrive className="h-4 w-4 shrink-0 text-primary" />
        <select
          className="h-8 rounded-[var(--radius)] border border-input bg-background px-2 text-sm"
 value={hostId}
          onChange={(e) => {
            const h = (hosts ?? []).find((x) => x.id === e.target.value);
            if (h) setHost(h.id, h.name);
          }}
        >
          {(hosts ?? []).map((h) => (
            <option key={h.id} value={h.id}>
              {h.name}
            </option>
          ))}
        </select>

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
        <Button variant="ghost" size="icon" className="h-8 w-8" onClick={() => refresh(cwd)} title="Refresh">
          <RefreshCw className={cn("h-4 w-4", loading && "animate-spin")} />
        </Button>

        <div className="mx-1 h-5 w-px bg-border" />

        <Button variant="ghost" size="sm" className="h-8" onClick={handleMkdir}>
          <FolderPlus className="h-4 w-4" /> {t("sftp.newFolder")}
        </Button>
        <Button variant="ghost" size="sm" className="h-8" onClick={() => fileInputRef.current?.click()}>
          <Upload className="h-4 w-4" /> {t("sftp.upload")}
        </Button>
        <input
          ref={fileInputRef}
          type="file"
          className="hidden"
          onChange={(e) => {
            const f = e.target.files?.[0];
            if (f) handleUpload(f);
            e.target.value = "";
          }}
        />
      </div>

      {/* Breadcrumb */}
      <div className="flex items-center gap-0.5 border-b border-border px-3 py-1 text-xs text-muted-foreground">
        {splitPath(cwd).map((part, i, arr) => {
          const path = arr.slice(0, i + 1).join("/") || "/";
          return (
            <span key={i} className="flex items-center">
              {i > 0 && <ChevronRight className="mx-0.5 h-3 w-3" />}
              <button
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
        {loading && entries.length === 0 ? (
          <div className="flex items-center justify-center py-12 text-sm text-muted-foreground">
            <Loader2 className="mr-2 h-4 w-4 animate-spin" /> Loading…
          </div>
        ) : entries.length === 0 ? (
          <div className="flex items-center justify-center py-12 text-sm text-muted-foreground">
            Empty directory
          </div>
        ) : (
          <table className="w-full text-sm">
            <tbody>
              {entries.map((entry) => (
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
                    <div className="flex justify-end gap-0.5 opacity-0 transition-opacity group-hover:opacity-100">
                      {!entry.isDir && (
                        <button
                          className="rounded p-1 hover:bg-accent"
                          title="Download"
                          onClick={(e) => {
                            e.stopPropagation();
                            handleDownload(entry);
                          }}
                        >
                          <Download className="h-3.5 w-3.5" />
                        </button>
                      )}
                      <button
                        className="rounded p-1 hover:bg-accent"
                        title="Rename"
                        onClick={(e) => {
                          e.stopPropagation();
                          startRename(entry);
                        }}
                      >
                        <Pencil className="h-3.5 w-3.5" />
                      </button>
                      <button
                        className="rounded p-1 text-destructive hover:bg-accent"
                        title="Delete"
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

      {/* Status bar */}
      <div className="flex items-center justify-between border-t border-border bg-card px-3 py-1 text-xs text-muted-foreground">
        <span>
          {hostName} · {entries.length} {t("sftp.items")}
        </span>
        {loading && <Badge variant="secondary">refreshing…</Badge>}
      </div>
    </div>
  );
}

function HostSelector({
  hosts,
  onSelect,
}: {
  hosts: { id: string; name: string }[];
  onSelect: (id: string, name: string) => void;
}) {
  return (
    <div className="flex flex-col gap-1">
      {hosts.map((h) => (
        <button
          key={h.id}
          className="flex items-center gap-2 rounded-[var(--radius)] border border-border bg-card px-3 py-2 text-left text-sm hover:border-primary/40 hover:bg-accent/40"
          onClick={() => onSelect(h.id, h.name)}
        >
          <HardDrive className="h-4 w-4 text-primary" />
          {h.name}
        </button>
      ))}
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
