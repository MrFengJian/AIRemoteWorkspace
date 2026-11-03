package mcpserver

import (
	"fmt"
	"sort"
	"strings"

	"github.com/ai-remote/workspace/internal/domain"
)

// LLM-facing rendering of the monitor data behind host_stats. Compact plain
// text, same shapes the app's monitor panel (and the diagnosis snapshot)
// show; pure functions, unit-tested in server_test.go.

func formatOverview(hostName string, o domain.MonitorOverview) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Host %s — overview\n", hostName)

	if o.CPUModel != "" || o.CPUCores > 0 {
		fmt.Fprintf(&b, "CPU: %.1f%%", o.CPUPercent)
		if o.CPUCores > 0 {
			fmt.Fprintf(&b, " (%d cores)", o.CPUCores)
		}
		if model := strings.TrimSpace(o.CPUModel); model != "" {
			fmt.Fprintf(&b, " — %s", model)
		}
		b.WriteString("\n")
	} else {
		fmt.Fprintf(&b, "CPU: %.1f%%\n", o.CPUPercent)
	}

	fmt.Fprintf(&b, "Load: %.2f %.2f %.2f", o.Load1, o.Load5, o.Load15)
	if o.UptimeSeconds > 0 {
		fmt.Fprintf(&b, "  uptime %s", fmtUptime(o.UptimeSeconds))
	}
	b.WriteString("\n")

	fmt.Fprintf(&b, "Memory: %s / %s used (%.1f%%)", fmtKB(o.MemUsedKB), fmtKB(o.MemTotalKB), o.MemUsedPercent)
	if o.SwapTotalKB > 0 {
		fmt.Fprintf(&b, "  swap %s / %s", fmtKB(o.SwapUsedKB), fmtKB(o.SwapTotalKB))
	}
	b.WriteString("\n")

	if len(o.Disks) > 0 {
		disks := make([]domain.MonitorDiskUsage, len(o.Disks))
		copy(disks, o.Disks)
		sort.Slice(disks, func(i, j int) bool { return disks[i].UsedPercent > disks[j].UsedPercent })
		b.WriteString("Disks (busiest first):\n")
		shown := 0
		for _, d := range disks {
			if shown >= 8 {
				fmt.Fprintf(&b, "  … %d more\n", len(disks)-shown)
				break
			}
			shown++
			fmt.Fprintf(&b, "  %s on %s: %.0f%% (%s of %s)\n",
				d.Device, d.Mount, d.UsedPercent, fmtKB(d.UsedKB), fmtKB(d.TotalKB))
		}
	}

	fmt.Fprintf(&b, "Network: ↓ %s  ↑ %s\n", fmtRate(o.NetRxBytesPerSec), fmtRate(o.NetTxBytesPerSec))

	fmt.Fprintf(&b, "Processes: %d total, %d running\n", o.ProcessTotal, o.ProcessRunning)

	if len(o.TCPStates) > 0 {
		states := make([]domain.MonitorTCPState, len(o.TCPStates))
		copy(states, o.TCPStates)
		sort.Slice(states, func(i, j int) bool { return states[i].Count > states[j].Count })
		parts := make([]string, 0, len(states))
		for _, s := range states {
			parts = append(parts, fmt.Sprintf("%s=%d", s.State, s.Count))
		}
		fmt.Fprintf(&b, "TCP: %s\n", strings.Join(parts, ", "))
	}

	if o.Kernel != "" {
		fmt.Fprintf(&b, "Kernel: %s\n", o.Kernel)
	}
	return strings.TrimRight(b.String(), "\n")
}

// topProcesses sorts a copy of procs and caps it at n entries.
func topProcesses(procs []domain.MonitorProcess, less func(a, b domain.MonitorProcess) bool, n int) []domain.MonitorProcess {
	sorted := make([]domain.MonitorProcess, len(procs))
	copy(sorted, procs)
	sort.Slice(sorted, func(i, j int) bool { return less(sorted[i], sorted[j]) })
	if len(sorted) > n {
		sorted = sorted[:n]
	}
	return sorted
}

func formatProcesses(hostName string, procs []domain.MonitorProcess) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Host %s — top processes\n", hostName)
	if len(procs) == 0 {
		b.WriteString("(no process data collected)")
		return b.String()
	}

	renderRows := func(header string, rows []domain.MonitorProcess) {
		b.WriteString(header + ":\n")
		for _, p := range rows {
			fmt.Fprintf(&b, "  %-7d %5.1f%% cpu  %-8s mem  %s", p.PID, p.CPUPercent, fmtKB(p.RSSKB), p.Name)
			if cmd := p.CommandLine; cmd != "" && cmd != p.Name {
				// One line per process; long command lines are truncated.
				line := " — " + cmd
				if len(line) > 120 {
					line = line[:117] + "..."
				}
				b.WriteString(line)
			}
			b.WriteString("\n")
		}
	}

	renderRows("By CPU", topProcesses(procs, func(a, b domain.MonitorProcess) bool { return a.CPUPercent > b.CPUPercent }, 15))
	renderRows("By memory", topProcesses(procs, func(a, b domain.MonitorProcess) bool { return a.RSSKB > b.RSSKB }, 5))
	return strings.TrimRight(b.String(), "\n")
}

func formatPorts(hostName string, ports []domain.MonitorPort) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Host %s — listening ports\n", hostName)
	if len(ports) == 0 {
		b.WriteString("(no listening TCP sockets found)")
		return b.String()
	}
	sorted := make([]domain.MonitorPort, len(ports))
	copy(sorted, ports)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Port != sorted[j].Port {
			return sorted[i].Port < sorted[j].Port
		}
		return sorted[i].Address < sorted[j].Address
	})
	shown := 0
	for _, p := range sorted {
		if shown >= 32 {
			fmt.Fprintf(&b, "… %d more\n", len(sorted)-shown)
			break
		}
		shown++
		addr := p.Address
		if addr == "0.0.0.0" || addr == "::" {
			addr = "*"
		}
		count := ""
		if p.Count > 1 {
			count = fmt.Sprintf(" (x%d)", p.Count)
		}
		fmt.Fprintf(&b, "  %s:%d %s%s\n", addr, p.Port, p.Proto, count)
	}
	return strings.TrimRight(b.String(), "\n")
}

// fmtKB renders kilobytes in B/K/M/G/T units.
func fmtKB(kb uint64) string {
	const k = 1024
	switch {
	case kb >= k*k*k:
		return fmt.Sprintf("%.1fT", float64(kb)/(k*k*k))
	case kb >= k*k:
		return fmt.Sprintf("%.1fG", float64(kb)/(k*k))
	case kb >= k:
		return fmt.Sprintf("%.1fM", float64(kb)/k)
	case kb > 0:
		return fmt.Sprintf("%.0fK", float64(kb))
	default:
		return "0B"
	}
}

// fmtRate renders bytes-per-second as B/s → KB/s → MB/s → GB/s.
func fmtRate(bps float64) string {
	switch {
	case bps >= 1024*1024*1024:
		return fmt.Sprintf("%.2fGB/s", bps/(1024*1024*1024))
	case bps >= 1024*1024:
		return fmt.Sprintf("%.1fMB/s", bps/(1024*1024))
	case bps >= 1024:
		return fmt.Sprintf("%.1fKB/s", bps/1024)
	default:
		return fmt.Sprintf("%.0fB/s", bps)
	}
}

// fmtUptime renders seconds as compact d/h/m.
func fmtUptime(sec float64) string {
	total := int(sec)
	d := total / 86400
	h := (total % 86400) / 3600
	m := (total % 3600) / 60
	switch {
	case d > 0:
		return fmt.Sprintf("%dd%dh", d, h)
	case h > 0:
		return fmt.Sprintf("%dh%dm", h, m)
	default:
		return fmt.Sprintf("%dm", m)
	}
}
