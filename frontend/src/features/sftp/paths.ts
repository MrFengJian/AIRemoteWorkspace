// Path helpers shared by the SFTP panel's local and remote panes. Remote
// paths are always POSIX; local paths follow the app machine's OS (the pane
// detects Windows-style paths by shape, which covers drives, backslashes and
// UNC shares).

import type { Side } from "@/features/sftp/clipboard";

/** Whether a path is Windows-shaped (drive letter or backslash anywhere). */
export function isWindowsPath(p: string): boolean {
  return /^[A-Za-z]:[\\/]/.test(p) || /^[A-Za-z]:$/.test(p) || p.includes("\\");
}

/** The separator that fits this directory's shape. */
function sepFor(dir: string): string {
  return isWindowsPath(dir) ? "\\" : "/";
}

/** Join a directory and an entry name for the given side. */
export function joinPath(side: Side, dir: string, name: string): string {
  if (side === "remote") {
    return (dir === "/" ? "" : dir.replace(/\/+$/, "")) + "/" + name;
  }
  const sep = sepFor(dir);
  const base = dir.replace(/[\\/]+$/, "");
  return base === "" ? sep + name : base + sep + name;
}

/** The parent of a directory (root maps to itself). */
export function parentPath(side: Side, dir: string): string {
  if (side === "remote") {
    return dir.replace(/\/[^/]+\/?$/, "") || "/";
  }
  const sep = sepFor(dir);
  const trimmed = dir.replace(/[\\/]+$/, "");
  const idx = trimmed.lastIndexOf(sep);
  if (idx < 0) return dir; // already at a root ("C:\", "/", or bare name)
  return trimmed.slice(0, idx + 1) || dir;
}

/** One clickable breadcrumb segment. */
export interface Crumb {
  label: string;
  path: string;
}

/**
 * Breadcrumb segments for a directory. rootLabel names the first segment
 * ("/" or the drive); later segments accumulate.
 */
export function crumbs(side: Side, cwd: string, rootLabel: string): Crumb[] {
  if (side === "remote" || !isWindowsPath(cwd)) {
    const parts = cwd.split("/").filter(Boolean);
    return [
      { label: rootLabel, path: "/" },
      ...parts.map((p, i) => ({
        label: p,
        path: "/" + parts.slice(0, i + 1).join("/"),
      })),
    ];
  }
  // Windows: "C:\Users\me" → C:\ | Users | me (drive keeps its root slash).
  const parts = cwd.replace(/\//g, "\\").split("\\").filter(Boolean);
  return parts.map((p, i) => {
    if (i === 0) {
      return { label: p, path: p.endsWith(":") ? p + "\\" : p };
    }
    return { label: p, path: parts.slice(0, i + 1).join("\\") + "\\" };
  });
}
