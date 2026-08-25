package domain

// Monitor data models. Metrics are collected over an existing SSH session by
// reading the remote /proc filesystem (and df for filesystem usage) — no
// externally-installed tools required; every command used (cat, grep, awk,
// df, tr, getconf, sleep) is part of the base system on mainstream distros
// and busybox alike.

// MonitorDiskUsage is one mounted filesystem's usage.
type MonitorDiskUsage struct {
	Device      string  `json:"device"`
	Mount       string  `json:"mount"`
	TotalKB     uint64  `json:"totalKb"`
	UsedKB      uint64  `json:"usedKb"`
	UsedPercent float64 `json:"usedPercent"`
}

// MonitorTCPState is one TCP connection-state histogram bucket.
type MonitorTCPState struct {
	State string `json:"state"` // "ESTABLISHED", "LISTEN", "TIME_WAIT", …
	Count int    `json:"count"`
}

// MonitorOverview is the host snapshot shown in the monitor panel's
// Overview tab. Rates are computed from two samples taken ~1s apart.
type MonitorOverview struct {
	CPUPercent   float64 `json:"cpuPercent"`
	CPUModel     string  `json:"cpuModel"`
	CPUCores     int     `json:"cpuCores"`
	Load1        float64 `json:"load1"`
	Load5        float64 `json:"load5"`
	Load15       float64 `json:"load15"`
	UptimeSeconds float64 `json:"uptimeSeconds"`
	Kernel       string  `json:"kernel"`

	MemTotalKB     uint64  `json:"memTotalKb"`
	MemUsedKB      uint64  `json:"memUsedKb"`
	MemUsedPercent float64 `json:"memUsedPercent"`
	SwapTotalKB    uint64  `json:"swapTotalKb"`
	SwapUsedKB     uint64  `json:"swapUsedKb"`

	Disks []MonitorDiskUsage `json:"disks"`

	NetRxBytesPerSec float64 `json:"netRxBytesPerSec"`
	NetTxBytesPerSec float64 `json:"netTxBytesPerSec"`

	ProcessTotal   int                `json:"processTotal"`
	ProcessRunning int                `json:"processRunning"`
	TCPStates      []MonitorTCPState  `json:"tcpStates"`
}

// MonitorProcess is one remote process with a live (sampled) CPU usage.
type MonitorProcess struct {
	PID         int     `json:"pid"`
	Name        string  `json:"name"`
	CommandLine string  `json:"commandLine"`
	CPUPercent  float64 `json:"cpuPercent"`
	RSSKB       uint64  `json:"rssKb"`
}

// MonitorPort is one listening TCP socket (or a group of identical binds).
type MonitorPort struct {
	Proto   string `json:"proto"`   // "tcp" | "tcp6"
	Address string `json:"address"` // decoded bind address
	Port    int    `json:"port"`
	Count   int    `json:"count"` // sockets with this identical bind (SO_REUSEPORT etc.)
}
