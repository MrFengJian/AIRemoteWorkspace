/** 简体中文 UI 文案，按功能模块分组。 */
export const zh = {
  // ── 通用 ────────────────────────────────────────────────────
  common: {
    loading: "加载中…",
    save: "保存",
    create: "创建",
    cancel: "取消",
    delete: "删除",
    edit: "编辑",
    connect: "连接",
    close: "关闭",
    refresh: "刷新",
    confirm: "确认",
    yes: "是",
    no: "否",
  },

  // ── 导航 ────────────────────────────────────────────────────
  nav: {
    dashboard: "仪表盘",
    hosts: "主机",
    terminal: "终端",
    files: "文件",
    agent: "助手",
    settings: "设置",
  },

  // ── 仪表盘 ──────────────────────────────────────────────────
  dashboard: {
    title: "AI 远程工作台",
    subtitle:
      "轻量、本地优先、AI 原生的桌面远程工作台。连接服务器、运行终端、管理文件，让 AI 助手诊断你的基础设施——全部在一个单文件应用里。",
    openPanel: "打开面板",
    hostManagement: {
      title: "主机管理",
      desc: "添加、编辑、测试和组织远程机器。",
      phase: "Phase 2",
    },
    sshTerminal: {
      title: "SSH 终端",
      desc: "基于 PTY 的稳定 SSH 终端（xterm.js）。",
      phase: "Phase 2",
    },
    fileWorkspace: {
      title: "文件工作区",
      desc: "浏览、上传、下载和编辑远程文件。",
      phase: "Phase 3",
    },
    aiAgent: {
      title: "AI 助手",
      desc: "LLM + 工具调用，带权限控制的执行。",
      phase: "Phase 4",
    },
  },

  // ── 主机 ────────────────────────────────────────────────────
  hosts: {
    title: "主机",
    subtitle: "可连接的远程机器。",
    addHost: "添加主机",
    searchPlaceholder: "按名称、IP、分组或标签搜索…",
    noMatch: '没有匹配 "{{query}}" 的主机。',
    noHostsTitle: "还没有主机",
    noHostsDesc: "添加一台服务器来管理它——连接、打开终端、浏览文件。",
    addFirst: "添加第一台主机",
    ungrouped: "未分组",
    doubleClickHint: "双击打开",
    authType: {
      password: "密码",
      key: "私钥文件",
      agent: "ssh-agent",
    },
    group: {
      // 仅 "none" 是 UI 语义；用户选择/输入的分组值（test/stage/production/自定义）
      // 原样存储显示，不做国际化。
      none: "无",
    },
    deleteConfirm: '删除主机 "{{name}}"？同时会清除已记住的密码。',
  },

  // ── 主机表单 ────────────────────────────────────────────────
  hostForm: {
    editTitle: "编辑主机",
    addTitle: "添加主机",
    description:
      "配置远程机器。凭据仅本次会话使用，除非勾选「记住」——届时存储到系统密码库，绝不存入数据库。",
    connection: "连接",
    name: "名称",
    host: "主机",
    username: "用户名",
    port: "端口",
    authentication: "认证方式",
    keyPath: "私钥路径（保存）",
    organisation: "组织",
    group: "分组",
    customGroup: "自定义分组",
    terminalScheme: "终端配色方案",
    schemeDefault: "默认（跟随设置）",
    tags: "标签",
    tagPlaceholder: "输入标签后按回车…",
    credentials: "凭据",
    password: "密码",
    keyOverride: "密钥文件（覆盖）",
    passphrase: "密码短语（如加密）",
    agentHint: "通过 SSH_AUTH_SOCK 使用 ssh-agent。无需输入凭据。",
    rememberPassword: "记住密码",
    rememberPassphrase: "记住密码短语",
    saved: "已保存",
    test: "测试",
    testConnected: "✓ 连接成功",
    connectOpen: "连接并打开终端",
    deleteHost: "删除",
  },

  // ── 终端 ────────────────────────────────────────────────────
  terminal: {
    noTerminals: "没有活跃的终端",
    noTerminalsDesc: "在主机面板打开一台主机以启动 SSH 终端会话。",
    goToHosts: "前往主机",
    sessionClosed: "会话已关闭 — 关闭",
    sessionExited: "[会话已退出]",
  },

  // ── 文件 ────────────────────────────────────────────────────
  sftp: {
    title: "文件",
    subtitle: "通过 SFTP 浏览和管理远程文件。",
    selectHost: "选择一台主机浏览其文件：",
    noHosts: "还没有主机——先在主机面板添加一台。",
    up: "上级",
    newFolder: "新建文件夹",
    upload: "上传",
    root: "根",
    empty: "空目录",
    items: "个项目",
    deleteConfirm: '删除 "{{name}}"？',
    newFolderPrompt: "新文件夹名称：",
  },

  // ── 助手 ────────────────────────────────────────────────────
  agent: {
    title: "AI 助手",
    noSession: "请先打开终端会话——助手操作已连接的主机。",
    llmConfig: "LLM 配置",
    noKey: "无密钥",
    placeholderConfigured: "让助手诊断或操作这台主机…",
    placeholderNoKey: "请先配置 LLM API 密钥才能使用助手…",
    send: "发送",
    stop: "停止",
    emptyHint: "让助手诊断这台主机——例如「检查 CPU 和内存」。",
    configTitle: "LLM 配置",
    configDesc: "配置 OpenAI 兼容的 API 提供商。API 密钥存储在系统密码库，绝不存入数据库。",
    baseUrl: "Base URL",
    model: "模型",
    apiKey: "API 密钥",
    apiKeyHint: "留空保留现有密钥。输入新密钥则替换。",
    approvalTitle: "需要批准",
    approvalDangerous: "危险操作",
    approvalDesc: "助手要执行 {{permission}} 操作。请仔细审查后再批准。",
    approvalArgs: "参数",
    approvalCommand: "命令",
    approvalPath: "路径",
    approvalFile: "文件",
    approvalDanger: "\u26A0 此操作可能造成不可逆损害。仅在你了解后果时才批准。",
    approve: "批准",
    deny: "拒绝",
    codeShell: "shell",
    codeCommand: "命令",
    codeScript: "脚本",
    copy: "复制",
    copied: "已复制",
    insert: "插入",
    inserted: "已插入",
    insertHint: "插入到终端会话",
    cancelled: "已取消",
    errorPrefix: "\u26A0\uFE0F",
  },

  // ── 设置 ────────────────────────────────────────────────────
  settings: {
    title: "设置",
    subtitle: "应用配置，本地存储。",
    securityMode: "安全模式",
    securityModeDesc: "控制凭据的存储方式和连接是否自动连接。",
    terminalScheme: "终端配色方案",
    terminalSchemeDesc: "为 SSH 终端会话选择配色方案。新会话立即使用所选方案。",
    selected: "已选",
    light: "（浅色）",
    runtime: "运行时",
    runtimeDesc: "本地 shell 和主题偏好。",
    defaultShell: "默认 shell",
    theme: "主题",
    loadError: "无法从后端加载/保存配置（{{error}}）。显示默认值。",
    loaded: "已从本地存储加载。",
    language: "语言",
    languageDesc: "界面语言。更改立即生效。",
    english: "English",
    chinese: "简体中文",
  },

  // ── 安全模式 ────────────────────────────────────────────────
  security: {
    convenience: "便利",
    balanced: "平衡（默认）",
    secure: "安全",
  },

  // ── 状态栏 ──────────────────────────────────────────────────
  status: {
    connecting: "连接中…",
  },
};
