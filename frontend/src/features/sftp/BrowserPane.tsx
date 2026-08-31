// BrowserPane — one file browser of the SFTP panel (local or remote). The
// two panes are identical in UI: compact toolbar, breadcrumb, listing table
// and a right-click menu. Cross-pane behavior (copy/paste/upload/download)
// is delegated to the parent via callbacks; dialogs for mkdir/delete live
// here because they are pure UI concerns.

import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import {
  ArrowLeft,
  Copy,
  Eye,
  EyeOff,
  File as FileIcon,
  Folder,
  FolderOpen,
  FolderPlus,
  Loader2,
  Pencil,
  RefreshCw,
  Trash2,
  Upload,
} from "lucide-react";
import type { LucideIcon } from "lucide-react";

import { Input } from "@/components/ui/input";
import { ContextMenu, type MenuItem } from "@/components/ui/ContextMenu";
import type { FileEntryDTO } from "@/features/sftp/api";
import type { BrowserState } from "@/features/sftp/useBrowser";
import { crumbs, joinPath, parentPath } from "@/features/sftp/paths";
import { formatSize } from "@/features/sftp/format";
import { useConfirm } from "@/lib/useConfirm";
import { toast } from "@/lib/toast";
import { cn } from "@/lib/utils";

interface BrowserPaneProps {
  browser: BrowserState;
  /** Pane label chip ("本地" / "远程"). */
  label: string;
  labelIcon: LucideIcon;
  /** A transfer is running — blocks mutating actions. */
  busy: boolean;
  /** The clipboard holds a pasteable entry for this pane. */
  pasteEnabled: boolean;
  /** Toolbar upload button (remote pane only — dialog-driven upload). */
  onUpload?: () => void;
  onMkdir: (name: string) => void;
  onCopy: (entry: FileEntryDTO) => void;
  /** Paste into targetDir (pane cwd or a right-clicked directory). */
  onPaste: (targetDir: string) => void;
  onRename: (oldName: string, newName: string) => void;
  onDelete: (entry: FileEntryDTO) => void;
  /** Row cross-pane action: upload (local pane) / download (remote pane). */
  onTransfer?: (entry: FileEntryDTO) => void;
  transferIcon?: LucideIcon;
  /** Label for the row cross-pane action ("上传" / "下载"). */
  transferLabel?: string;
  /** Double-click on a file (remote pane transfers; local pane: no-op). */
  onOpenFile?: (entry: FileEntryDTO) => void;
}

export function BrowserPane({
  browser,
  label,
  labelIcon: LabelIcon,
  busy,
  pasteEnabled,
  onUpload,
  onMkdir,
  onCopy,
  onPaste,
  onRename,
  onDelete,
  onTransfer,
  transferIcon: TransferIcon,
  transferLabel,
  onOpenFile,
}: BrowserPaneProps) {
  const { t } = useTranslation();
  const { askConfirm, askPrompt } = useConfirm();

  const [pathInput, setPathInput] = useState(browser.cwd);
  const [renaming, setRenaming] = useState<string | null>(null);
  const [renameValue, setRenameValue] = useState("");
  /** Right-click menu: null = closed; entry null = background menu. */
  const [menu, setMenu] = useState<{ x: number; y: number; entry: FileEntryDTO | null } | null>(null);

  // Keep the path input in sync with navigation from anywhere.
  useEffect(() => setPathInput(browser.cwd), [browser.cwd]);

  const entryFullPath = (name: string) => joinPath(browser.side, browser.cwd, name);

  const submitPath = () => {
    const p = pathInput.trim();
    if (p && p !== browser.cwd) browser.navigate(p);
  };

  /** Copy text (entry name / full path) to the OS clipboard. */
  const copyText = async (text: string) => {
    try {
      await navigator.clipboard.writeText(text);
      toast.info(t("sftp.copied"));
    } catch {
      toast.error(t("sftp.copyFailed"));
    }
  };

  const promptMkdir = async () => {
    const name = await askPrompt({
      title: t("sftp.newFolderTitle"),
      placeholder: t("sftp.newFolderPrompt"),
      confirmLabel: t("sftp.newFolder"),
    });
    if (name?.trim()) onMkdir(name.trim());
  };

  const confirmDelete = async (entry: FileEntryDTO) => {
    const ok = await askConfirm({
      title: t("sftp.deleteTitle"),
      message: t("sftp.deleteConfirm", { name: entry.name }),
      danger: true,
      confirmLabel: t("common.delete"),
    });
    if (ok) onDelete(entry);
  };

  const commitRename = (oldName: string) => {
    if (renameValue && renameValue !== oldName) onRename(oldName, renameValue);
    setRenaming(null);
  };

  const openEntry = (entry: FileEntryDTO) => {
    if (entry.isDir) {
      browser.navigate(entryFullPath(entry.name));
    } else {
      onOpenFile?.(entry);
    }
  };

  /** Context-menu items for a row entry, or the background when entry is
   *  null. Copy/paste sit above the existing name/path text-copy group. */
  const buildMenuItems = (entry: FileEntryDTO | null): MenuItem[] => {
    if (!entry) {
      const items: MenuItem[] = [
        {
          label: t("sftp.paste"),
          icon: Copy,
          disabled: !pasteEnabled || busy,
          onClick: () => onPaste(browser.cwd),
        },
        { type: "separator" },
      ];
      if (onUpload) {
        items.push({ label: t("sftp.upload"), icon: Upload, onClick: () => onUpload(), disabled: busy });
      }
      items.push(
        { label: t("sftp.newFolder"), icon: FolderPlus, onClick: () => void promptMkdir(), disabled: busy },
        { label: t("common.refresh"), icon: RefreshCw, onClick: () => browser.refresh() },
        { type: "separator" },
        {
          label: browser.showHidden ? t("sftp.hideHidden") : t("sftp.showHidden"),
          icon: browser.showHidden ? EyeOff : Eye,
          checked: browser.showHidden,
          onClick: () => browser.setShowHidden(!browser.showHidden),
        },
      );
      return items;
    }

    const items: MenuItem[] = [];
    if (entry.isDir) {
      items.push({ label: t("sftp.open"), icon: FolderOpen, onClick: () => openEntry(entry) });
    } else if (onTransfer && TransferIcon) {
      items.push({ label: transferLabel ?? t("sftp.transferTitle"), icon: TransferIcon, onClick: () => onTransfer(entry), disabled: busy });
    }
    items.push(
      { type: "separator" },
      { label: t("sftp.copy"), icon: Copy, onClick: () => onCopy(entry) },
    );
    if (entry.isDir) {
      items.push({
        label: t("sftp.paste"),
        icon: Copy,
        disabled: !pasteEnabled || busy,
        onClick: () => onPaste(entryFullPath(entry.name)),
      });
    }
    items.push(
      { type: "separator" },
      {
        label: t(entry.isDir ? "sftp.copyDirName" : "sftp.copyFileName"),
        icon: Copy,
        onClick: () => void copyText(entry.name),
      },
      { label: t("sftp.copyPath"), icon: Copy, onClick: () => void copyText(entryFullPath(entry.name)) },
      { type: "separator" },
      { label: t("sftp.rename"), icon: Pencil, onClick: () => startRename(entry), disabled: busy },
      { label: t("common.delete"), icon: Trash2, danger: true, onClick: () => void confirmDelete(entry), disabled: busy },
    );
    return items;
  };

  const startRename = (entry: FileEntryDTO) => {
    setRenaming(entry.name);
    setRenameValue(entry.name);
  };

  // Dotfiles ("." and ".." never come through; entries like ".bashrc") are
  // filtered client-side so the toggle is instant — no refetch needed.
  const visibleEntries = browser.showHidden
    ? browser.entries
    : browser.entries.filter((e) => !e.name.startsWith("."));

  const crumbsList = crumbs(browser.side, browser.cwd, t("sftp.root"));

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      {/* Toolbar */}
      <div className="flex h-8 shrink-0 items-center gap-1 border-b border-border bg-card px-1.5">
        <button
          type="button"
          className="flex h-6 w-6 shrink-0 items-center justify-center rounded transition-colors hover:bg-accent"
          onClick={() => browser.navigate(parentPath(browser.side, browser.cwd))}
          title={t("sftp.up")}
        >
          <ArrowLeft className="h-3.5 w-3.5" />
        </button>
        <Input
          className="h-6 min-w-0 flex-1 px-1.5 font-mono text-xs"
          value={pathInput}
          onChange={(e) => setPathInput(e.target.value)}
          onKeyDown={(e) => e.key === "Enter" && submitPath()}
          placeholder="/"
        />
        <button
          type="button"
          className="flex h-6 w-6 shrink-0 items-center justify-center rounded transition-colors hover:bg-accent"
          onClick={() => browser.refresh()}
          disabled={busy}
          title={t("common.refresh")}
        >
          <RefreshCw className={cn("h-3.5 w-3.5", browser.loading && "animate-spin")} />
        </button>
        <button
          type="button"
          className={cn(
            "flex h-6 w-6 shrink-0 items-center justify-center rounded transition-colors hover:bg-accent",
            browser.showHidden && "bg-accent text-primary",
          )}
          onClick={() => browser.setShowHidden(!browser.showHidden)}
          title={browser.showHidden ? t("sftp.hideHidden") : t("sftp.showHidden")}
        >
          {browser.showHidden ? <Eye className="h-3.5 w-3.5" /> : <EyeOff className="h-3.5 w-3.5" />}
        </button>
        <button
          type="button"
          className="flex h-6 w-6 shrink-0 items-center justify-center rounded transition-colors hover:bg-accent"
          onClick={() => void promptMkdir()}
          disabled={busy}
          title={t("sftp.newFolder")}
        >
          <FolderPlus className="h-3.5 w-3.5" />
        </button>
        {onUpload && (
          <button
            type="button"
            className="flex h-6 w-6 shrink-0 items-center justify-center rounded transition-colors hover:bg-accent"
            onClick={() => onUpload()}
            disabled={busy}
            title={t("sftp.upload")}
          >
            {busy ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Upload className="h-3.5 w-3.5" />}
          </button>
        )}
      </div>

      {/* Breadcrumb + pane label */}
      <div className="flex shrink-0 items-center gap-0.5 overflow-x-auto border-b border-border px-2 py-0.5 text-xs text-muted-foreground">
        <span className="mr-1 flex shrink-0 items-center gap-1 rounded bg-secondary px-1.5 py-px text-[10px] font-medium text-secondary-foreground">
          <LabelIcon className="h-3 w-3" />
          {label}
        </span>
        {crumbsList.map((c, i) => (
          <span key={i} className="flex shrink-0 items-center">
            {i > 0 && <span className="mx-0.5 opacity-60">/</span>}
            <button
              type="button"
              className="max-w-[8rem] truncate rounded px-1 hover:bg-accent hover:text-foreground"
              onClick={() => browser.navigate(c.path)}
            >
              {c.label}
            </button>
          </span>
        ))}
      </div>

      {/* Error banner */}
      {browser.error && (
        <div className="shrink-0 border-b border-destructive/30 bg-destructive/10 px-3 py-1 text-xs text-destructive">
          {browser.error}
        </div>
      )}

      {/* File list — right-click opens the pane's own context menu (rows stop
          propagation so the background menu only fires on empty space; the
          browser default is suppressed everywhere). */}
      <div
        className="min-h-0 flex-1 overflow-auto"
        onContextMenu={(e) => {
          e.preventDefault();
          setMenu({ x: e.clientX, y: e.clientY, entry: null });
        }}
      >
        {browser.loading && visibleEntries.length === 0 ? (
          <div className="flex items-center justify-center py-8 text-sm text-muted-foreground">
            <Loader2 className="mr-2 h-4 w-4 animate-spin" /> {t("common.loading")}
          </div>
        ) : visibleEntries.length === 0 ? (
          <div className="flex items-center justify-center py-8 text-sm text-muted-foreground">
            {t("sftp.empty")}
          </div>
        ) : (
          <table className="w-full text-sm">
            <thead>
              <tr className="sticky top-0 z-[1] border-b border-border bg-card text-left text-xs text-muted-foreground">
                <th className="w-7 py-1 pl-2.5" aria-label={t("sftp.colName")} />
                <th className="py-1 pr-2 font-medium">{t("sftp.colName")}</th>
                <th className="hidden w-20 py-1 pr-2 text-right font-medium sm:table-cell">
                  {t("sftp.colSize")}
                </th>
                <th className="hidden w-32 py-1 pr-2 font-medium md:table-cell">
                  {t("sftp.colModified")}
                </th>
                <th className="w-20 py-1 pr-1.5" aria-label={t("common.edit")} />
              </tr>
            </thead>
            <tbody>
              {visibleEntries.map((entry) => (
                <tr
                  key={entry.name}
                  className="group border-b border-border/50 hover:bg-accent/30"
                  onDoubleClick={() => openEntry(entry)}
                  onContextMenu={(e) => {
                    e.preventDefault();
                    e.stopPropagation();
                    setMenu({ x: e.clientX, y: e.clientY, entry });
                  }}
                >
                  <td className="w-7 py-1 pl-2.5">
                    {entry.isDir ? (
                      <Folder className="h-3.5 w-3.5 text-primary" />
                    ) : (
                      <FileIcon className="h-3.5 w-3.5 text-muted-foreground" />
                    )}
                  </td>
                  <td className="max-w-0 py-1 pr-2">
                    {renaming === entry.name ? (
                      <Input
                        autoFocus
                        className="h-5 px-1 py-0 text-xs"
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
                        className="block max-w-full truncate text-left text-foreground"
                        onClick={() => openEntry(entry)}
                        title={entry.name}
                      >
                        {entry.name}
                      </button>
                    )}
                  </td>
                  <td className="hidden w-20 py-1 pr-2 text-right font-mono text-xs text-muted-foreground sm:table-cell">
                    {entry.isDir ? "—" : formatSize(entry.size)}
                  </td>
                  <td className="hidden w-32 py-1 pr-2 text-xs text-muted-foreground md:table-cell">
                    {entry.modTime ? new Date(entry.modTime).toLocaleString() : ""}
                  </td>
                  <td className="w-20 py-1 pr-1.5 text-right">
                    <div className="flex justify-end gap-0.5 opacity-0 transition-opacity focus-within:opacity-100 group-hover:opacity-100">
                      {onTransfer && TransferIcon && !entry.isDir && (
                        <button
                          type="button"
                          className="rounded p-1 hover:bg-accent"
                          title={transferLabel ?? t("sftp.transferTitle")}
                          onClick={(e) => {
                            e.stopPropagation();
                            onTransfer(entry);
                          }}
                        >
                          <TransferIcon className="h-3 w-3" />
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
                        <Pencil className="h-3 w-3" />
                      </button>
                      <button
                        type="button"
                        className="rounded p-1 text-destructive hover:bg-accent"
                        title={t("common.delete")}
                        onClick={(e) => {
                          e.stopPropagation();
                          void confirmDelete(entry);
                        }}
                      >
                        <Trash2 className="h-3 w-3" />
                      </button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      {/* Status footer: item count + hidden count + refreshing badge */}
      <div className="flex shrink-0 items-center justify-between border-t border-border bg-card px-2 py-0.5 text-[11px] text-muted-foreground">
        <span>
          {visibleEntries.length} {t("sftp.items")}
          {!browser.showHidden && browser.entries.length > visibleEntries.length && (
            <span className="ml-1">
              {t("sftp.hiddenCount", { count: browser.entries.length - visibleEntries.length })}
            </span>
          )}
        </span>
        {browser.loading && <span className="flex items-center gap-1"><Loader2 className="h-3 w-3 animate-spin" />{t("sftp.refreshing")}</span>}
      </div>

      {/* Context menu (row entry or panel background) */}
      {menu && (
        <ContextMenu
          x={menu.x}
          y={menu.y}
          items={buildMenuItems(menu.entry)}
          onClose={() => setMenu(null)}
        />
      )}
    </div>
  );
}
