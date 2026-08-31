// Formatting helpers for the SFTP panel.

/** Basename for both POSIX and Windows paths. */
export function baseName(p: string): string {
  return p.split(/[\\/]/).pop() || p;
}

/** Split "report.tar.gz" into base + last extension (dotfiles stay whole). */
export function splitExt(name: string): { base: string; ext: string } {
  const i = name.lastIndexOf(".");
  if (i <= 0) return { base: name, ext: "" };
  return { base: name.slice(0, i), ext: name.slice(i) };
}

/**
 * First non-conflicting name: name → name(1).ext → name(2).ext … The
 * generator keeps counting so the renamed file can't collide either.
 */
export async function suggestName(
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

export function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  if (bytes < 1024 * 1024 * 1024) return `${(bytes / 1024 / 1024).toFixed(1)} MB`;
  return `${(bytes / 1024 / 1024 / 1024).toFixed(1)} GB`;
}
