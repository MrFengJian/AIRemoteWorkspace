// Buffer content highlighting for terminal panes — ONE unified pipeline:
// every highlight (built-in HTTP/ERROR/WARN and user-defined) is a rule
// {regex, color, underline?}. Rules are scanned over the xterm buffer and
// matches painted with buffer decorations in the rule's color scheme
// (always visible — not the hover-only link layer).
//
// Scanning is incremental: the panel calls scanNew() after each output write,
// so every buffer line is regex-scanned exactly once; settings changes force
// a full rescan. CJK-aware: match columns are converted from character
// indexes to cell indexes (double-width chars occupy two cells).

import type { IDecoration, Terminal } from "@xterm/xterm";

export interface HighlightRuleCompiled {
  re: RegExp;
  color: string; // palette id (HL_COLORS)
  underline?: boolean;
}

export interface HighlightOptions {
  rules: HighlightRuleCompiled[];
}

/** Highlight color palette (ids double as i18n suffixes in the settings UI).
 *  Backgrounds are translucent so the glyphs stay readable on any theme. */
export const HL_COLORS: Record<string, { bg: string }> = {
  red: { bg: "rgba(248, 113, 113, 0.30)" },
  orange: { bg: "rgba(249, 115, 22, 0.30)" },
  yellow: { bg: "rgba(250, 204, 21, 0.26)" },
  green: { bg: "rgba(34, 197, 94, 0.28)" },
  cyan: { bg: "rgba(34, 211, 238, 0.26)" },
  blue: { bg: "rgba(59, 130, 246, 0.30)" },
  purple: { bg: "rgba(168, 85, 247, 0.28)" },
  pink: { bg: "rgba(236, 72, 153, 0.28)" },
};

/** Palette ids in display order. */
export const HL_COLOR_IDS = Object.keys(HL_COLORS);

// Built-in rule patterns — same regex+color rules as user-defined ones,
// gated by the two settings toggles.
const URL_RE = /https?:\/\/[^\s\x00-\x1f"'`<>(){}`[\]\\]+/gi;
const ERROR_RE = /\b(ERROR|ERRORS|FATAL|CRITICAL|FAILED|FAILURE|FAILURES)\b/g;
const WARN_RE = /\b(WARN|WARNING|WARNINGS)\b/g;

/** The built-in rule set: HTTP/HTTPS links (cyan), hard failures (red),
 *  warnings (yellow). */
export function builtinRules(links: boolean, keywords: boolean): HighlightRuleCompiled[] {
  const out: HighlightRuleCompiled[] = [];
  if (links) out.push({ re: URL_RE, color: "cyan", underline: true });
  if (keywords) {
    out.push({ re: ERROR_RE, color: "red" });
    out.push({ re: WARN_RE, color: "yellow" });
  }
  return out;
}

/** Effective rule list: built-ins (per toggles) first, then user rules. */
export function buildRules(
  links: boolean,
  keywords: boolean,
  user: HighlightRuleCompiled[],
): HighlightRuleCompiled[] {
  return [...builtinRules(links, keywords), ...user];
}

/** Compile user patterns to regexes; invalid ones are dropped (never break
 *  the terminal over a bad rule). */
export function compileRules(
  rules: Array<{ pattern?: string; color?: string }> | null | undefined,
): HighlightRuleCompiled[] {
  const out: HighlightRuleCompiled[] = [];
  for (const r of rules ?? []) {
    if (!r?.pattern || !r.color) continue;
    try {
      out.push({ re: new RegExp(r.pattern, "g"), color: r.color });
    } catch {
      /* invalid regex — skip */
    }
  }
  return out;
}

// Decoration budget — a pathological output (a dumped JSON blob full of
// URLs) must not register thousands of overlay elements.
const MAX_DECORATIONS = 2000;

/** Styles applied to the decoration overlay element on every render. */
function applyStyle(el: HTMLElement, color: string, underline?: boolean): void {
  el.style.pointerEvents = "none"; // never block text selection
  const c = HL_COLORS[color];
  if (c) el.style.backgroundColor = c.bg;
  if (underline) {
    el.style.textDecoration = "underline";
    el.style.textUnderlineOffset = "2px";
    el.style.color = "#7dd3fc";
  }
}

/** Collect non-overlapping matches on one line of plain text. Overlaps
 *  resolve to the leftmost match (longest wins on ties). */
export function findMatches(
  text: string,
  rules: HighlightRuleCompiled[],
): Array<{ col: number; len: number; color: string; underline?: boolean }> {
  const raw: Array<{ s: number; e: number; color: string; underline?: boolean }> = [];
  for (const rule of rules) {
    rule.re.lastIndex = 0;
    for (let m = rule.re.exec(text); m; m = rule.re.exec(text)) {
      if (m[0]) raw.push({ s: m.index, e: m.index + m[0].length, color: rule.color, underline: rule.underline });
    }
  }
  raw.sort((a, b) => a.s - b.s || b.e - a.e);
  const out: Array<{ col: number; len: number; color: string; underline?: boolean }> = [];
  let end = -1;
  for (const m of raw) {
    if (m.s < end) continue;
    out.push({ col: m.s, len: m.e - m.s, color: m.color, underline: m.underline });
    end = m.e;
  }
  return out;
}

export class BufferHighlighter {
  private term: Terminal;
  private opts: HighlightOptions;
  private decorations: Array<{ dec: IDecoration; disposeMarker: () => void }> = [];
  /** Absolute buffer lines already scanned. */
  private scanned = 0;

  constructor(term: Terminal, opts: HighlightOptions) {
    this.term = term;
    this.opts = opts;
  }

  /** Apply new settings; a change triggers a full buffer rescan. */
  update(opts: HighlightOptions): void {
    const changed =
      JSON.stringify(this.opts.rules.map((r) => [r.re.source, r.color, r.underline ?? false])) !==
      JSON.stringify(opts.rules.map((r) => [r.re.source, r.color, r.underline ?? false]));
    this.opts = opts;
    if (changed) this.scanAll();
  }

  /** Scan buffer lines appended since the last call (cheap, per write). */
  scanNew(): void {
    const total = this.term.buffer.active.length;
    if (total < this.scanned) {
      // Buffer was trimmed (clear / scrollback reset) — start over.
      this.clear();
      this.scanned = 0;
    }
    if (total > this.scanned) {
      this.scanRange(this.scanned, total);
      this.scanned = total;
    }
  }

  scanAll(): void {
    this.clear();
    this.scanned = 0;
    this.scanNew();
  }

  clear(): void {
    for (const d of this.decorations) {
      d.dec.dispose();
      d.disposeMarker();
    }
    this.decorations = [];
  }

  dispose(): void {
    this.clear();
  }

  private scanRange(from: number, to: number): void {
    if (this.opts.rules.length === 0) return;
    const buf = this.term.buffer.active;
    const cursorAbs = buf.baseY + this.term.rows - 1;
    for (let i = from; i < to; i++) {
      if (this.decorations.length >= MAX_DECORATIONS) return;
      const line = buf.getLine(i);
      if (!line) continue;
      const text = line.translateToString(true);
      if (!text) continue;
      const matches = findMatches(text, this.opts.rules);
      if (matches.length === 0) continue;

      // Character index → cell index map (built lazily; a wide char is one
      // string char but two cells, which would otherwise skew match widths).
      const charToCell: number[] = [];
      for (let x = 0; x < line.length; x++) {
        const cell = line.getCell(x);
        if (!cell || cell.getWidth() === 0) continue; // right half of a wide char
        if (cell.getChars() === "") break; // past end of content
        charToCell.push(x);
      }

      for (const m of matches) {
        const col = charToCell[m.col] ?? m.col;
        const endCol = charToCell[m.col + m.len - 1];
        const width = endCol !== undefined ? endCol - col + 1 : m.len;
        const marker = this.term.registerMarker(i - cursorAbs);
        if (!marker) continue;
        const dec = this.term.registerDecoration({ marker, x: col, width });
        if (!dec) {
          marker.dispose();
          continue;
        }
        dec.onRender((el) => applyStyle(el, m.color, m.underline));
        this.decorations.push({ dec, disposeMarker: () => marker.dispose() });
      }
    }
  }
}
