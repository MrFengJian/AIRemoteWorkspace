/**
 * Keybinding parsing and matching. Bindings use a canonical string form:
 * modifiers in fixed order (Ctrl, Alt, Shift, Meta) followed by one key
 * name — "Ctrl+Shift+C", "Ctrl+Tab", "Ctrl+=", "Shift+Insert".
 *
 * KeyboardEvents are normalized to the same form (physical-key based where
 * layouts disagree: digits and = / - come from e.code so Shift+= records as
 * "Shift+=" rather than "Shift++").
 */

export interface ParsedBinding {
  ctrl: boolean;
  alt: boolean;
  shift: boolean;
  meta: boolean;
  /** Canonical key name: "C", "Tab", "=", "1", "PageDown", "F5", … */
  key: string;
}

/** Accepted spellings → canonical key name (lowercased input). */
const KEY_ALIASES: Record<string, string> = {
  escape: "Escape",
  esc: "Escape",
  enter: "Enter",
  return: "Enter",
  space: "Space",
  tab: "Tab",
  backspace: "Backspace",
  insert: "Insert",
  ins: "Insert",
  delete: "Delete",
  del: "Delete",
  pagedown: "PageDown",
  pgdn: "PageDown",
  pageup: "PageUp",
  pgup: "PageUp",
  home: "Home",
  end: "End",
  arrowup: "Up",
  up: "Up",
  arrowdown: "Down",
  down: "Down",
  arrowleft: "Left",
  left: "Left",
  arrowright: "Right",
  right: "Right",
  equal: "=",
  equals: "=",
  minus: "-",
  dash: "-",
  comma: ",",
  period: ".",
  slash: "/",
  backslash: "\\",
  semicolon: ";",
  quote: "'",
  backquote: "`",
  bracketleft: "[",
  bracketright: "]",
};

const MODIFIERS = new Set(["ctrl", "control", "alt", "option", "shift", "meta", "cmd", "command", "win", "super"]);

/** Canonical display name for a held-modifier event key ("Control" → "Ctrl"). */
const MODIFIER_EVENT_KEYS = new Set(["Control", "Alt", "Shift", "Meta"]);

function normalizeKey(raw: string): string | null {
  const lower = raw.toLowerCase();
  if (KEY_ALIASES[lower]) return KEY_ALIASES[lower];
  if (/^f([1-9]|1[0-9]|2[0-4])$/.test(lower)) return `F${lower.slice(1)}`; // F1–F24
  if (/^[a-z]$/.test(lower)) return lower.toUpperCase();
  if (/^digit\d$/.test(lower)) return lower.slice(5); // "digit1" → "1"
  if (raw.length === 1) return raw; // symbols as-is: = , . / …
  return null;
}

/** Parse a binding string; null when malformed (no key, or two key parts). */
export function parseBinding(binding: string): ParsedBinding | null {
  const parts = binding
    .split("+")
    .map((p) => p.trim())
    .filter((p) => p.length > 0);
  if (parts.length === 0) return null;

  let ctrl = false;
  let alt = false;
  let shift = false;
  let meta = false;
  let key: string | null = null;
  for (const part of parts) {
    const lower = part.toLowerCase();
    if (lower === "ctrl" || lower === "control") ctrl = true;
    else if (lower === "alt" || lower === "option") alt = true;
    else if (lower === "shift") shift = true;
    else if (MODIFIERS.has(lower)) meta = true; // meta / cmd / command / win / super
    else {
      if (key !== null) return null;
      key = normalizeKey(part);
      if (!key) return null;
    }
  }
  if (!key) return null;
  return { ctrl, alt, shift, meta, key };
}

export function formatBinding(p: ParsedBinding): string {
  const parts: string[] = [];
  if (p.ctrl) parts.push("Ctrl");
  if (p.alt) parts.push("Alt");
  if (p.shift) parts.push("Shift");
  if (p.meta) parts.push("Meta");
  parts.push(p.key);
  return parts.join("+");
}

/**
 * Normalize a KeyboardEvent to the canonical binding string. Returns null
 * for events that cannot form a binding: IME composition, dead keys, and
 * bare modifier presses (the recorder shows those as a partial "Ctrl+…").
 */
export function describeEvent(e: KeyboardEvent): string | null {
  if (e.isComposing || e.key === "Dead" || e.key === "Process") return null;

  let key = normalizeKey(e.key);
  // Layout-stable disambiguation: e.key for digits and = / - depends on the
  // active layout and Shift state ("!" for Shift+1, "+" for Shift+= on US).
  // e.code always reflects the physical key.
  const code = e.code || "";
  if (/^Digit\d$/.test(code)) key = code.slice(5);
  else if (code === "Equal") key = "=";
  else if (code === "Minus") key = "-";
  else if (code === "NumpadAdd") key = "+";
  else if (code === "NumpadSubtract") key = "-";
  else if (code === "NumpadEnter") key = "Enter";
  else if (code === "NumpadMultiply") key = "*";
  else if (code === "NumpadDivide") key = "/";

  if (!key || MODIFIER_EVENT_KEYS.has(e.key)) return null;

  return formatBinding({
    ctrl: e.ctrlKey,
    alt: e.altKey,
    shift: e.shiftKey,
    meta: e.metaKey,
    key,
  });
}

/** Keys that are safe to bind without a Ctrl/Alt/Meta modifier: function
 *  and navigation keys never produce typed characters, so a bare binding
 *  can't fire while the user is typing (Xshell's Shift+Insert style). */
const STANDALONE_OK = /^(F([1-9]|1[0-9]|2[0-4])|Insert|Delete|PageUp|PageDown|Home|End|Up|Down|Left|Right)$/;

/** Validation for the shortcut recorder: printable keys (letters, digits,
 *  symbols, Enter, Space, Tab) require at least one of Ctrl/Alt/Meta. */
export function isBindingAllowed(binding: string): boolean {
  const p = parseBinding(binding);
  if (!p) return false;
  if (p.ctrl || p.alt || p.meta) return true;
  return STANDALONE_OK.test(p.key);
}
