/**
 * Terminal colour schemes. The `theme` field matches the ITheme interface
 * from @xterm/xterm. Values are the most popular schemes in the developer
 * community (matched against iTerm2 / VS Code theme registries).
 *
 * The `id` is what gets persisted in AppConfig.terminalTheme — adding a new
 * scheme here + an entry in TERMINAL_THEMES makes it selectable.
 */
export interface TerminalThemeDef {
  id: string;
  label: string;
  /** Whether this is a light background scheme (affects the tab UI contrast). */
  light?: boolean;
  theme: {
    background: string;
    foreground: string;
    cursor: string;
    cursorAccent?: string;
    selectionBackground?: string;
    black: string;
    red: string;
    green: string;
    yellow: string;
    blue: string;
    magenta: string;
    cyan: string;
    white: string;
    brightBlack: string;
    brightRed: string;
    brightGreen: string;
    brightYellow: string;
    brightBlue: string;
    brightMagenta: string;
    brightCyan: string;
    brightWhite: string;
  };
}

export const TERMINAL_THEMES: TerminalThemeDef[] = [
  {
    id: "cobalt2",
    label: "Cobalt2",
    theme: {
      background: "#193549",
      foreground: "#ffffff",
      cursor: "#ff628c",
      cursorAccent: "#193549",
      selectionBackground: "#3a6ea5",
      black: "#000000",
      red: "#ff628c",
      green: "#3ad900",
      yellow: "#ffc600",
      blue: "#0082c9",
      magenta: "#fb94ff",
      cyan: "#9effff",
      white: "#ffffff",
      brightBlack: "#0050a4",
      brightRed: "#ff628c",
      brightGreen: "#3ad900",
      brightYellow: "#ffc600",
      brightBlue: "#0082c9",
      brightMagenta: "#fb94ff",
      brightCyan: "#9effff",
      brightWhite: "#ffffff",
    },
  },
  {
    id: "dracula",
    label: "Dracula",
    theme: {
      background: "#282a36",
      foreground: "#f8f8f2",
      cursor: "#f8f8f0",
      selectionBackground: "#44475a",
      black: "#000000",
      red: "#ff5555",
      green: "#50fa7b",
      yellow: "#f1fa8c",
      blue: "#bd93f9",
      magenta: "#ff79c6",
      cyan: "#8be9fd",
      white: "#bfbfbf",
      brightBlack: "#4d4d4d",
      brightRed: "#ff6e67",
      brightGreen: "#5af78e",
      brightYellow: "#f4f99d",
      brightBlue: "#caa9fa",
      brightMagenta: "#ff92d0",
      brightCyan: "#9aedfe",
      brightWhite: "#e6e6e6",
    },
  },
  {
    id: "solarized-dark",
    label: "Solarized Dark",
    theme: {
      background: "#002b36",
      foreground: "#839496",
      cursor: "#93a1a1",
      selectionBackground: "#073642",
      black: "#073642",
      red: "#dc322f",
      green: "#859900",
      yellow: "#b58900",
      blue: "#268bd2",
      magenta: "#d33682",
      cyan: "#2aa198",
      white: "#eee8d5",
      brightBlack: "#002b36",
      brightRed: "#cb4b16",
      brightGreen: "#586e75",
      brightYellow: "#657b83",
      brightBlue: "#839496",
      brightMagenta: "#6c71c4",
      brightCyan: "#93a1a1",
      brightWhite: "#fdf6e3",
    },
  },
  {
    id: "one-dark",
    label: "One Dark",
    theme: {
      background: "#282c34",
      foreground: "#abb2bf",
      cursor: "#528bff",
      selectionBackground: "#3e4451",
      black: "#5c6370",
      red: "#e06c75",
      green: "#98c379",
      yellow: "#e5c07b",
      blue: "#61afef",
      magenta: "#c678dd",
      cyan: "#56b6c2",
      white: "#abb2bf",
      brightBlack: "#5c6370",
      brightRed: "#e06c75",
      brightGreen: "#98c379",
      brightYellow: "#e5c07b",
      brightBlue: "#61afef",
      brightMagenta: "#c678dd",
      brightCyan: "#56b6c2",
      brightWhite: "#ffffff",
    },
  },
  {
    id: "nord",
    label: "Nord",
    theme: {
      background: "#2e3440",
      foreground: "#d8dee9",
      cursor: "#d8dee9",
      selectionBackground: "#434c5e",
      black: "#3b4252",
      red: "#bf616a",
      green: "#a3be8c",
      yellow: "#ebcb8b",
      blue: "#81a1c1",
      magenta: "#b48ead",
      cyan: "#88c0d0",
      white: "#e5e9f0",
      brightBlack: "#4c566a",
      brightRed: "#bf616a",
      brightGreen: "#a3be8c",
      brightYellow: "#ebcb8b",
      brightBlue: "#81a1c1",
      brightMagenta: "#b48ead",
      brightCyan: "#8fbcbb",
      brightWhite: "#eceff4",
    },
  },
  {
    id: "github-light",
    label: "GitHub Light",
    light: true,
    theme: {
      background: "#ffffff",
      foreground: "#24292e",
      cursor: "#044289",
      selectionBackground: "#c8c8fa",
      black: "#24292e",
      red: "#d73a49",
      green: "#28a745",
      yellow: "#dbab09",
      blue: "#0366d6",
      magenta: "#5a32a3",
      cyan: "#0598bc",
      white: "#6a737d",
      brightBlack: "#959da5",
      brightRed: "#cb2431",
      brightGreen: "#22863a",
      brightYellow: "#b08800",
      brightBlue: "#005cc5",
      brightMagenta: "#5a32a3",
      brightCyan: "#3192aa",
      brightWhite: "#d1d5da",
    },
  },
];

/** Default theme id — must match domain.DefaultConfig().TerminalTheme. */
export const DEFAULT_TERMINAL_THEME = "cobalt2";

/** Look up a theme by id, falling back to the default. */
export function getTerminalTheme(id: string | undefined | null): TerminalThemeDef {
  return (
    TERMINAL_THEMES.find((t) => t.id === id) ??
    TERMINAL_THEMES.find((t) => t.id === DEFAULT_TERMINAL_THEME)!
  );
}
