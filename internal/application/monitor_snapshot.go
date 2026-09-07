package application

import (
	"context"
	"fmt"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/ai-remote/workspace/internal/domain"
)

// Snapshot aggregates one deterministic health check round for the host
// behind sessionID (the diagnosis agent's triage context): CPU / memory /
// disk / load, top processes, listening ports, and recent error-log lines.
// Everything is read-only and collected with the same zero-dependency
// channel as the monitor panel (remote: /proc over the session's exec;
// local: native/sh tools). The output is a compact plain-text report meant
// to be injected into the first turn of a diagnosis conversation — sections
// that fail individually are skipped instead of failing the whole snapshot;
// only the overview is required.
func (s *MonitorService) Snapshot(ctx context.Context, sessionID string) (string, error) {
	overview, err := s.GetOverview(ctx, sessionID)
	if err != nil {
		return "", fmt.Errorf("snapshot overview: %w", err)
	}

	procs, perr := s.GetProcesses(ctx, sessionID)
	ports, _ := s.GetPorts(ctx, sessionID)
	errLogs := s.collectErrorLogs(ctx, sessionID)

	return buildSnapshotText(time.Now(), overview, procs, perr == nil, ports, errLogs), nil
}

// errorLogScript pulls the recent error evidence in one round: failed systemd
// units, the last error-level journal lines, and kernel-ring lines that look
// problematic (grep instead of dmesg --level for busybox compatibility).
// All read-only; empty output on hosts without systemd/journal is fine.
const errorLogScript = `echo =FAILED
systemctl list-units --state=failed --no-legend --plain 2>/dev/null | head -10
echo =JOURNAL
journalctl -p err -n 25 --no-pager 2>/dev/null | tail -25
echo =DMESG
dmesg -T 2>/dev/null | tail -60 | grep -iE "err|fail|oom|out of memory|killed process|reset|timeout" | tail -15`

// collectErrorLogs gathers the journal/dmesg/failed-units section. Remote
// sessions run the script over the SSH session; local unix sessions run it
// through sh; other local platforms (Windows) skip the section.
func (s *MonitorService) collectErrorLogs(ctx context.Context, sessionID string) string {
	if isLocalSessionID(sessionID) {
		if runtime.GOOS == "windows" {
			return ""
		}
		out, _, err := s.collectLocal(ctx, errorLogScript)
		if err != nil {
			return ""
		}
		return out
	}
	out, _, err := s.collect(ctx, sessionID, errorLogScript)
	if err != nil {
		return ""
	}
	return out
}

// buildSnapshotText renders the collected pieces as a compact report. Pure
// function (unit-tested): procsOK distinguishes "no processes" from
// "collection failed", and errLogs is raw =SECTION script output.
func buildSnapshotText(now time.Time, o domain.MonitorOverview, procs []domain.MonitorProcess, procsOK bool, ports []domain.MonitorPort, errLogs string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Health snapshot @ %s (auto-collected, read-only)\n", now.Format("2006-01-02 15:04:05"))

	if o.Kernel != "" || o.CPUModel != "" {
		fmt.Fprintf(&b, "Host: %s", strings.TrimSpace(o.CPUModel))
		if o.CPUCores > 0 {
			fmt.Fprintf(&b, ", %d cores", o.CPUCores)
		}
		if o.Kernel != "" {
			fmt.Fprintf(&b, ", kernel %s", o.Kernel)
		}
		b.WriteString("\n")
	}
	if o.UptimeSeconds > 0 {
		fmt.Fprintf(&b, "Uptime: %s\n", humanDuration(o.UptimeSeconds))
	}
	fmt.Fprintf(&b, "CPU: %.1f%%  load: %.2f %.2f %.2f\n", o.CPUPercent, o.Load1, o.Load5, o.Load15)
	fmt.Fprintf(&b, "Memory: %s of %s (%.1f%%)", humanKB(o.MemUsedKB), humanKB(o.MemTotalKB), o.MemUsedPercent)
	if o.SwapTotalKB > 0 {
		fmt.Fprintf(&b, "  swap: %s of %s", humanKB(o.SwapUsedKB), humanKB(o.SwapTotalKB))
	}
	b.WriteString("\n")

	if len(o.Disks) > 0 {
		b.WriteString("Disks:\n")
		disks := make([]domain.MonitorDiskUsage, len(o.Disks))
		copy(disks, o.Disks)
		sort.Slice(disks, func(i, j int) bool { return disks[i].UsedPercent > disks[j].UsedPercent })
		for i, d := range disks {
			if i >= 8 {
				break
			}
			if d.Mount == "/tmp" || strings.HasPrefix(d.Mount, "/snap/") || strings.Contains(d.Device, "tmpfs") {
				continue
			}
			fmt.Fprintf(&b, "  %s on %s: %.0f%% (%s of %s)\n", d.Device, d.Mount, d.UsedPercent, humanKB(d.UsedKB), humanKB(d.TotalKB))
		}
	}

	if procsOK && len(procs) > 0 {
		byCPU := make([]domain.MonitorProcess, len(procs))
		byMem := make([]domain.MonitorProcess, len(procs))
		copy(byCPU, procs)
		copy(byMem, procs)
		sort.Slice(byCPU, func(i, j int) bool { return byCPU[i].CPUPercent > byCPU[j].CPUPercent })
		sort.Slice(byMem, func(i, j int) bool { return byMem[i].RSSKB > byMem[j].RSSKB })
		b.WriteString("Top processes by CPU:\n")
		for i, p := range byCPU {
			if i >= 5 {
				break
			}
			fmt.Fprintf(&b, "  %d %.1f%% cpu %s mem  %s\n", p.PID, p.CPUPercent, humanKB(p.RSSKB), firstField(p.CommandLine))
		}
		b.WriteString("Top processes by memory:\n")
		for i, p := range byMem {
			if i >= 3 {
				break
			}
			fmt.Fprintf(&b, "  %d %s mem %s  %s\n", p.PID, humanKB(p.RSSKB), shortName(p.Name), firstField(p.CommandLine))
		}
	}

	if len(ports) > 0 {
		sort.Slice(ports, func(i, j int) bool { return ports[i].Port < ports[j].Port })
		items := make([]string, 0, len(ports))
		for i, p := range ports {
			if i >= 16 {
				items = append(items, "…")
				break
			}
			addr := p.Address
			if addr == "0.0.0.0" || addr == "::" {
				addr = "*"
			}
			items = append(items, fmt.Sprintf("%s:%d", addr, p.Port))
		}
		fmt.Fprintf(&b, "Listening ports: %s\n", strings.Join(items, ", "))
	}

	if failed, journal, dmesg := parseErrorSections(errLogs); failed != "" || journal != "" || dmesg != "" {
		b.WriteString("Recent errors:\n")
		if failed != "" {
			fmt.Fprintf(&b, "  failed units:\n%s\n", indent(failed, "    "))
		}
		if journal != "" {
			fmt.Fprintf(&b, "  journal (err, last lines):\n%s\n", indent(journal, "    "))
		}
		if dmesg != "" {
			fmt.Fprintf(&b, "  kernel ring:\n%s\n", indent(dmesg, "    "))
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// parseErrorSections splits the errorLogScript output back into its three
// blocks (all empty when the section never ran or came back empty).
func parseErrorSections(raw string) (failed, journal, dmesg string) {
	if strings.TrimSpace(raw) == "" {
		return "", "", ""
	}
	sec := sections(raw)
	failed = capLines(sec["FAILED"], 10)
	journal = capLines(sec["JOURNAL"], 25)
	dmesg = capLines(sec["DMESG"], 15)
	return
}

// capLines joins a section's lines, dropping empty ones.
func capLines(lines []string, max int) string {
	kept := make([]string, 0, len(lines))
	for _, l := range lines {
		if t := strings.TrimRight(l, "\r"); t != "" {
			kept = append(kept, t)
		}
		if len(kept) >= max {
			break
		}
	}
	return strings.Join(kept, "\n")
}

// indent prefixes every line (of non-empty text) with the given padding.
func indent(text, pad string) string {
	lines := strings.Split(text, "\n")
	for i, l := range lines {
		lines[i] = pad + l
	}
	return strings.Join(lines, "\n")
}

// humanKB renders kilobytes in B/K/M/G/T units.
func humanKB(kb uint64) string {
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

// humanDuration renders seconds as d/h/m compact form.
func humanDuration(sec float64) string {
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

// firstField returns the first whitespace-delimited token (the binary path).
func firstField(s string) string {
	if i := strings.IndexAny(s, " \t"); i > 0 {
		s = s[:i]
	}
	if len(s) > 80 {
		s = s[:80] + "…"
	}
	return s
}

// shortName trims a process name for the memory list.
func shortName(s string) string {
	if len(s) > 24 {
		return s[:24] + "…"
	}
	return s
}
