/** English UI strings, grouped by feature. */
export const en = {
  // ── Common ──────────────────────────────────────────────────
  common: {
    loading: "Loading…",
    save: "Save",
    create: "Create",
    cancel: "Cancel",
    delete: "Delete",
    edit: "Edit",
    connect: "Connect",
    close: "Close",
    refresh: "Refresh",
    confirm: "Confirm",
    yes: "Yes",
    no: "No",
  },

  // ── Navigation (Sidebar / TopBar) ───────────────────────────
  nav: {
    dashboard: "Dashboard",
    hosts: "Hosts",
    terminal: "Terminal",
    files: "Files",
    agent: "Agent",
    settings: "Settings",
  },

  // ── Dashboard ───────────────────────────────────────────────
  dashboard: {
    title: "AI Remote Workspace",
    subtitle:
      "A lightweight, local-first, AI-native desktop remote workspace. Connect machines, run terminals, manage files, and let an AI agent diagnose your infrastructure — all from a single binary.",
    openPanel: "Open panel",
    hostManagement: {
      title: "Host Management",
      desc: "Add, edit, test, and organise remote machines.",
      phase: "Phase 2",
    },
    sshTerminal: {
      title: "SSH Terminal",
      desc: "Stable PTY-backed terminal over SSH (xterm.js).",
      phase: "Phase 2",
    },
    fileWorkspace: {
      title: "File Workspace",
      desc: "Browse, upload, download, and edit remote files.",
      phase: "Phase 3",
    },
    aiAgent: {
      title: "AI Agent",
      desc: "LLM + Tool Calling with permission-gated execution.",
      phase: "Phase 4",
    },
  },

  // ── Hosts ───────────────────────────────────────────────────
  hosts: {
    title: "Hosts",
    subtitle: "Remote machines you can connect to.",
    addHost: "Add Host",
    searchPlaceholder: "Search by name, IP, group, or tag…",
    noMatch: 'No hosts match "{{query}}".',
    noHostsTitle: "No hosts yet",
    noHostsDesc:
      "Add a server to manage it — connect, open a terminal, and browse files.",
    addFirst: "Add your first host",
    ungrouped: "Ungrouped",
    doubleClickHint: "double-click to open",
    authType: {
      password: "Password",
      key: "Private key file",
      agent: "ssh-agent",
    },
    group: {
      // Only "none" is UI semantics; user-selected group values (test/stage/
      // production/custom) are stored and displayed verbatim, never localised.
      none: "None",
    },
    deleteConfirm: 'Delete host "{{name}}"? This also clears any remembered password.',
  },

  // ── Host form ───────────────────────────────────────────────
  hostForm: {
    editTitle: "Edit Host",
    addTitle: "Add Host",
    description:
      "Configure a remote machine. Credentials are used only for this session unless you tick \u201CRemember\u201D — then they\u2019re stored in the OS credential vault, never in the database.",
    connection: "Connection",
    name: "Name",
    host: "Host",
    username: "Username",
    port: "Port",
    authentication: "Authentication",
    keyPath: "Private key path (saved)",
    organisation: "Organisation",
    group: "Group",
    customGroup: "Custom group",
    terminalScheme: "Terminal colour scheme",
    schemeDefault: "Default (settings)",
    tags: "Tags",
    tagPlaceholder: "Type a tag and press Enter…",
    credentials: "Credentials",
    password: "Password",
    keyOverride: "Key file (override)",
    passphrase: "Passphrase (if encrypted)",
    agentHint: "Uses ssh-agent via SSH_AUTH_SOCK. No credential entry needed.",
    rememberPassword: "Remember password",
    rememberPassphrase: "Remember passphrase",
    saved: "saved",
    test: "Test",
    testConnected: "✓ Connected successfully",
    connectOpen: "Connect & Open Terminal",
    deleteHost: "Delete",
  },

  // ── Terminal ────────────────────────────────────────────────
  terminal: {
    noTerminals: "No active terminals",
    noTerminalsDesc: "Open a host in the Hosts panel to start an SSH terminal session.",
    goToHosts: "Go to Hosts",
    sessionClosed: "Session closed — dismiss",
    sessionExited: "[session exited]",
  },

  // ── SFTP ────────────────────────────────────────────────────
  sftp: {
    title: "Files",
    subtitle: "Browse and manage remote files over SFTP.",
    selectHost: "Select a host to browse its files:",
    noHosts: "No hosts yet — add one in the Hosts panel first.",
    up: "Up",
    newFolder: "New Folder",
    upload: "Upload",
    root: "root",
    empty: "Empty directory",
    items: "item(s)",
    deleteConfirm: 'Delete "{{name}}"?',
    newFolderPrompt: "New folder name:",
  },

  // ── Agent ───────────────────────────────────────────────────
  agent: {
    title: "AI Agent",
    noSession: "Open a terminal session first — the agent operates on the connected host.",
    llmConfig: "LLM Config",
    noKey: "no key",
    placeholderConfigured: "Ask the agent to diagnose or operate on this host…",
    placeholderNoKey: "Configure LLM API key first to use the agent…",
    send: "Send",
    stop: "Stop",
    emptyHint: "Ask the agent to diagnose this host — e.g. \u201Ccheck CPU and memory\u201D.",
    configTitle: "LLM Configuration",
    configDesc:
      "Configure an OpenAI-compatible API provider. The API key is stored in the OS credential vault, never in the database.",
    baseUrl: "Base URL",
    model: "Model",
    apiKey: "API Key",
    apiKeyHint: "Leave blank to keep the existing key. Enter a new key to replace it.",
    approvalTitle: "Approval Required",
    approvalDangerous: "Dangerous Operation",
    approvalDesc: "The agent wants to run a {{permission}} operation. Review carefully before approving.",
    approvalArgs: "Arguments",
    approvalCommand: "Command",
    approvalPath: "Path",
    approvalFile: "File",
    approvalDanger: "\u26A0 This operation can cause irreversible damage. Only approve if you understand the consequences.",
    approve: "Approve",
    deny: "Deny",
    codeShell: "shell",
    codeCommand: "command",
    codeScript: "script",
    copy: "Copy",
    copied: "Copied",
    insert: "Insert",
    inserted: "Inserted",
    insertHint: "Insert into terminal session",
    cancelled: "cancelled",
    errorPrefix: "\u26A0\uFE0F",
  },

  // ── Settings ────────────────────────────────────────────────
  settings: {
    title: "Settings",
    subtitle: "Application configuration, stored locally.",
    securityMode: "Security Mode",
    securityModeDesc:
      "Controls how credentials are stored and whether connections auto-connect.",
    terminalScheme: "Terminal Colour Scheme",
    terminalSchemeDesc:
      "Choose a colour scheme for SSH terminal sessions. New sessions use the selected scheme immediately.",
    selected: "Selected",
    light: "(light)",
    runtime: "Runtime",
    runtimeDesc: "Local shell and theme preferences.",
    defaultShell: "Default shell",
    theme: "Theme",
    loadError: "Could not load/save config from backend ({{error}}). Showing defaults.",
    loaded: "Loaded from local store.",
    language: "Language",
    languageDesc: "Interface language. Changes take effect immediately.",
    english: "English",
    chinese: "简体中文",
  },

  // ── Security modes ──────────────────────────────────────────
  security: {
    convenience: "Convenience",
    balanced: "Balanced (default)",
    secure: "Secure",
  },

  // ── Status bar ──────────────────────────────────────────────
  status: {
    connecting: "connecting…",
  },
};
