/**
 * Operating-system icon mapping for host display.
 *
 * Backend stores a distro id (e.g. "ubuntu", "rockylinux") on the host. Each
 * known id maps to a simple-icons SVG (served from /os/<id>.svg) plus a
 * human-friendly display name. Unknown ids fall back to the generic Linux icon.
 */

export interface OSInfo {
  /** Path to the SVG in /public/os. */
  icon: string;
  /** Display name, e.g. "Ubuntu". */
  label: string;
}

const ICON_BASE = "/os";

/** Known distro → { icon, label }. Fallback (linux) is always present. */
const OS_MAP: Record<string, OSInfo> = {
  ubuntu: { icon: "ubuntu", label: "Ubuntu" },
  debian: { icon: "debian", label: "Debian" },
  centos: { icon: "centos", label: "CentOS" },
  rockylinux: { icon: "rockylinux", label: "Rocky Linux" },
  almalinux: { icon: "almalinux", label: "AlmaLinux" },
  fedora: { icon: "fedora", label: "Fedora" },
  archlinux: { icon: "archlinux", label: "Arch Linux" },
  arch: { icon: "archlinux", label: "Arch Linux" },
  opensuse: { icon: "opensuse", label: "openSUSE" },
  suse: { icon: "opensuse", label: "SUSE" },
  alpinelinux: { icon: "alpinelinux", label: "Alpine Linux" },
  alpine: { icon: "alpinelinux", label: "Alpine" },
  redhat: { icon: "redhat", label: "Red Hat" },
  rhel: { icon: "redhat", label: "RHEL" },
  kalilinux: { icon: "kalilinux", label: "Kali Linux" },
  kali: { icon: "kalilinux", label: "Kali" },
  linuxmint: { icon: "linuxmint", label: "Linux Mint" },
  mint: { icon: "linuxmint", label: "Mint" },
  manjaro: { icon: "manjaro", label: "Manjaro" },
  elementaryos: { icon: "elementaryos", label: "elementary OS" },
  elementary: { icon: "elementaryos", label: "elementary OS" },
  freebsd: { icon: "freebsd", label: "FreeBSD" },
  windows: { icon: "windows", label: "Windows" },
  apple: { icon: "apple", label: "macOS" },
  darwin: { icon: "apple", label: "macOS" },
  macos: { icon: "apple", label: "macOS" },
  linux: { icon: "linux", label: "Linux" },
};

/** Resolve a backend OS id to display info; unknown ids → generic Linux. */
export function osInfo(id: string | undefined | null): OSInfo | null {
  if (!id) return null;
  const known = OS_MAP[id.toLowerCase()];
  if (known) {
    return { icon: `${ICON_BASE}/${known.icon}.svg`, label: known.label };
  }
  return { icon: `${ICON_BASE}/linux.svg`, label: "Linux" };
}
