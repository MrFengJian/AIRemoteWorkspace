// Buffer content highlighting for terminal panes: scans the xterm buffer for
// http/https links and ERROR/WARN-class log keywords, then paints matches
// with buffer decorations (always visible — not the hover-only link layer).
//
// Scanning is incremental: the panel calls scanNew() after each output write,
// so every buffer line is regex-scanned exactly once; settings changes force
// a full rescan. CJK-aware: match columns are converted from character
// indexes to cell indexes (double-width chars occupy two cells).

import type { IDecoration, Terminal } from "@xterm/xterm";

export interface HighlightOptions {
  links: boolean;
  keywords: boolean;
}

const URL_RE = /https?:\/\/[^\s\x00-\x1f"'`<>(){}`[\]\\]+/gi;
// Tier 1 (red): hard failures. Tier 2 (amber): warnings.
const ERROR_RE = /\b(ERROR|ERRORS|FATAL|CRITICAL|FAILED|FAILURE|FAILURES)\b/g;
const WARN_RE = /\b(WARN|WARNING|WARNINGS)\b/g;

// Decoration budget — a pathological output (a dumped JSON blob full of
// URLs) must not register thousands of overlay elements.
const MAX_DECORATIONS = 2000;

interface Match {
  col: number; // cell index of the match start
  len: number; // width in cells
  kind: "url" | "error" | "warn";
}

/** Styles applied to the decoration overlay element on every render. */
const STYLES: Record<Match["kind"], (el: HTMLElement) => void> = {
  url: (el) => {
    el.style.textDecoration = "underline";
    el.style.textUnderlineOffset = "2px";
    el.style.color = "#7dd3fc";
  },
  error: (el) => {
    el.style.backgroundColor = "rgba(248, 113, 113, 0.28)";
  },
  warn: (el) => {
    el.style.backgroundColor = "rgba(250, 204, 21, 0.24)";
  },
};

/** Collect non-overlapping matches on one line of plain text. */
export function findMatches(text: string, opts: HighlightOptions): Match[] {
  const raw: Array<{ s: number; e: number; kind: Match["kind"] }> = [];
  const run = (re: RegExp, kind: Match["kind"]) => {
    re.lastIndex = 0;
    for (let m = re.exec(text); m; m = re.exec(text)) {
      if (m[0]) raw.push({ s: m.index, e: m.index + m[0].length, kind });
    }
  };
  if (opts.links) run(URL_RE, "url");
  if (opts.keywords) {
    run(ERROR_RE, "error");
    run(WARN_RE, "warn");
  }
  raw.sort((a, b) => a.s - b.s || b.e - a.e);
  const out: Match[] = [];
  let end = -1;
  for (const m of raw) {
    if (m.s < end) continue; // overlaps an earlier match
    out.push({ col: m.s, len: m.e - m.s, kind: m.kind });
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

  /** Apply new settings; a toggle change triggers a full buffer rescan. */
  update(opts: HighlightOptions): void {
    const changed =
      opts.links !== this.opts.links || opts.keywords !== this.opts.keywords;
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
    if (!this.opts.links && !this.opts.keywords) return;
    const buf = this.term.buffer.active;
    const cursorAbs = buf.baseY + this.term.rows - 1;
    for (let i = from; i < to; i++) {
      if (this.decorations.length >= MAX_DECORATIONS) return;
      const line = buf.getLine(i);
      if (!line) continue;
      const text = line.translateToString(true);
      if (!text) continue;
      const matches = findMatches(text, this.opts);
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
        dec.onRender((el) => {
          el.style.pointerEvents = "none"; // never block text selection
          STYLES[m.kind](el);
        });
        this.decorations.push({ dec, disposeMarker: () => marker.dispose() });
      }
    }
  }
}
