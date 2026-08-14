/**
 * Terminal font options shared by Settings → Appearance and the per-host
 * override in the host form. Empty value = the built-in default stack.
 */
export const TERMINAL_FONTS = [
  { value: "", label: "Cascadia Mono (default)" },
  { value: "JetBrains Mono", label: "JetBrains Mono" },
  { value: "Cascadia Code", label: "Cascadia Code" },
  { value: "Fira Code", label: "Fira Code" },
  { value: "Source Code Pro", label: "Source Code Pro" },
  { value: "Consolas", label: "Consolas" },
  { value: "Menlo", label: "Menlo" },
  { value: "monospace", label: "monospace" },
];

/** xterm font-family value for a chosen font ("" = default stack). */
export function terminalFontFamily(font: string | undefined): string {
  return font
    ? `"${font}", "Cascadia Mono", Consolas, monospace`
    : '"Cascadia Mono", "JetBrains Mono", Consolas, Menlo, monospace';
}
