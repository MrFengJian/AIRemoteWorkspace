package application

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ai-remote/workspace/internal/domain"
)

// Local-host collection for LOCAL terminal sessions (the app's own machine).
// Routing mirrors TerminalService: session ids with the "local-" prefix
// belong to the localpty manager (the constant is repeated here to keep the
// application layer free of infrastructure imports).
//
//   - Linux reuses the SSH scripts unchanged through the local shell — the
//     same POSIX tools and /proc reads apply to the running machine.
//   - macOS collects through its own base-system tools (sysctl / vm_stat /
//     top / netstat / ps / df); /proc does not exist there.
//   - Windows has no lightweight native channel — collectLocal fails, and
//     the frontend shows an explanatory hint instead of a hard error.

func isLocalSessionID(sessionID string) bool {
	return strings.HasPrefix(sessionID, "local-")
}

func (s *MonitorService) collectLocal(ctx context.Context, script string) (string, float64, error) {
	start := time.Now()
	cmd := exec.CommandContext(ctx, "sh", "-c", script)
	out, err := cmd.CombinedOutput()
	elapsed := time.Since(start).Seconds()
	if err != nil {
		return "", 0, fmt.Errorf("monitor: local exec: %w", err)
	}
	return string(out), elapsed, nil
}

func (s *MonitorService) localOverview(ctx context.Context) (domain.MonitorOverview, error) {
	if runtime.GOOS == "darwin" {
		out, elapsed, err := s.collectLocal(ctx, darwinOverviewScript)
		if err != nil {
			return domain.MonitorOverview{}, err
		}
		return buildOverviewDarwin(out, elapsed)
	}
	out, elapsed, err := s.collectLocal(ctx, overviewScript)
	if err != nil {
		return domain.MonitorOverview{}, err
	}
	return buildOverview(out, elapsed)
}

func (s *MonitorService) localProcesses(ctx context.Context) ([]domain.MonitorProcess, error) {
	if runtime.GOOS == "darwin" {
		out, elapsed, err := s.collectLocal(ctx, darwinProcessScript)
		if err != nil {
			return nil, err
		}
		return buildProcessesDarwin(out, elapsed), nil
	}
	out, elapsed, err := s.collectLocal(ctx, processScript)
	if err != nil {
		return nil, err
	}
	return buildProcesses(out, elapsed), nil
}

func (s *MonitorService) localPorts(ctx context.Context) ([]domain.MonitorPort, error) {
	if runtime.GOOS == "darwin" {
		out, _, err := s.collectLocal(ctx, darwinPortScript)
		if err != nil {
			return nil, err
		}
		return buildPortsDarwin(out), nil
	}
	out, _, err := s.collectLocal(ctx, portScript)
	if err != nil {
		return nil, err
	}
	return buildPorts(out), nil
}

// ── macOS scripts (base-system tools only) ─────────────────────────────

const darwinOverviewScript = `echo =CPUINFO
sysctl -n machdep.cpu.brand_string 2>/dev/null
echo "cores $(sysctl -n hw.ncpu 2>/dev/null || echo 1)"
echo =STATIDLE
top -l 2 -n 0 -s 1 2>/dev/null | grep 'CPU usage' | tail -1
echo =NET1
netstat -ib 2>/dev/null | awk 'NR==1 { for (i=1; i<=NF; i++) { if ($i=="Ibytes") ib=i; if ($i=="Obytes") ob=i }; next } NR>1 && !seen[$1]++ && $1!="lo0" { print $1, $ib+0, $ob+0 }'
sleep 1
echo =NET2
netstat -ib 2>/dev/null | awk 'NR==1 { for (i=1; i<=NF; i++) { if ($i=="Ibytes") ib=i; if ($i=="Obytes") ob=i }; next } NR>1 && !seen[$1]++ && $1!="lo0" { print $1, $ib+0, $ob+0 }'
echo =MEM
sysctl -n hw.memsize 2>/dev/null
vm_stat 2>/dev/null
echo =SWAP
sysctl -n vm.swapusage 2>/dev/null
echo =LOAD
sysctl -n vm.loadavg 2>/dev/null
echo =BOOT
sysctl -n kern.boottime 2>/dev/null
echo =KERNEL
sysctl -n kern.osrelease 2>/dev/null
echo =DF
df -P -k 2>/dev/null
echo =TCP
netstat -an -p tcp 2>/dev/null | awk 'NR>1 { st = $NF; if (st ~ /^[A-Z][A-Z_0-9]*$/) s[st]++ } END { for (k in s) print k, s[k] }'
echo =PROC
ps -axo stat 2>/dev/null | awk 'NR>1 { n++; if (substr($1,1,1)=="R") r++ } END { print r+0, n+0 }'`

const darwinProcessScript = `echo =PS1
ps -axo pid,cputime,rss,comm 2>/dev/null | tail -n +2
sleep 1
echo =PS2
ps -axo pid,cputime 2>/dev/null | tail -n +2`

const darwinPortScript = `netstat -an -p tcp 2>/dev/null | awk '$NF=="LISTEN" {print $1, $4}'`

// ── macOS parsers ──────────────────────────────────────────────────────

var (
	darwinIdleRe   = regexp.MustCompile(`([0-9.]+)% idle`)
	darwinHdrPSRe  = regexp.MustCompile(`page size of (\d+) bytes`)
	darwinPageRe   = regexp.MustCompile(`^Pages (free|inactive|speculative|active|wired|purgeable):\s+(\d+)\.`)
	darwinSwapRe   = regexp.MustCompile(`total = ([0-9.]+)([KMG])?\s+used = ([0-9.]+)([KMG])?`)
	darwinBootRe   = regexp.MustCompile(`sec = (\d+)`)
)

func darwinSizeToKB(v float64, unit string) float64 {
	switch unit {
	case "G":
		return v * 1024 * 1024
	case "M":
		return v * 1024
	default:
		return v // KB / unitless
	}
}

func clampPercent(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}

func buildOverviewDarwin(out string, elapsedSec float64) (domain.MonitorOverview, error) {
	sec := sections(out)
	if len(sec) == 0 {
		return domain.MonitorOverview{}, errMonitorEmpty
	}
	o := domain.MonitorOverview{}

	// CPU: brand line + cores line; usage = 100 - idle from top's 2nd sample.
	for _, l := range sec["CPUINFO"] {
		if strings.HasPrefix(l, "cores ") {
			o.CPUCores, _ = strconv.Atoi(strings.TrimPrefix(l, "cores "))
		} else if o.CPUModel == "" {
			o.CPUModel = strings.TrimSpace(l)
		}
	}
	for _, l := range sec["STATIDLE"] {
		if m := darwinIdleRe.FindStringSubmatch(l); m != nil {
			if idle, err := strconv.ParseFloat(m[1], 64); err == nil {
				o.CPUPercent = round1(clampPercent(100 - idle))
			}
		}
	}

	// Memory: hw.memsize (bytes) + vm_stat page buckets (KB = pages × ps/1024).
	pageSize := 16384.0
	pages := map[string]float64{}
	memTotalBytes := 0.0
	for _, l := range sec["MEM"] {
		if m := darwinHdrPSRe.FindStringSubmatch(l); m != nil {
			if v, err := strconv.ParseFloat(m[1], 64); err == nil {
				pageSize = v
			}
			continue
		}
		if m := darwinPageRe.FindStringSubmatch(l); m != nil {
			if v, err := strconv.ParseFloat(m[2], 64); err == nil {
				pages[m[1]] = v
			}
			continue
		}
		if v, err := strconv.ParseFloat(strings.TrimSpace(l), 64); err == nil && v > 1e9 {
			memTotalBytes = v
		}
	}
	if memTotalBytes > 0 {
		o.MemTotalKB = uint64(memTotalBytes / 1024)
		available := (pages["free"] + pages["inactive"] + pages["speculative"]) * pageSize / 1024
		if float64(o.MemTotalKB) > available {
			o.MemUsedKB = uint64(float64(o.MemTotalKB) - available)
		}
		if o.MemTotalKB > 0 {
			o.MemUsedPercent = round1(float64(o.MemUsedKB) / float64(o.MemTotalKB) * 100)
		}
	}

	// Swap: "total = 2048.00M  used = 100.00M  free = 1948.00M".
	for _, l := range sec["SWAP"] {
		if m := darwinSwapRe.FindStringSubmatch(l); m != nil {
			total, _ := strconv.ParseFloat(m[1], 64)
			used, _ := strconv.ParseFloat(m[3], 64)
			o.SwapTotalKB = uint64(darwinSizeToKB(total, m[2]))
			o.SwapUsedKB = uint64(darwinSizeToKB(used, m[4]))
		}
	}

	// Load: "{ 1.92 1.87 1.90 }".
	for _, l := range sec["LOAD"] {
		f := strings.Fields(strings.Trim(l, "{} "))
		if len(f) >= 3 {
			o.Load1, _ = strconv.ParseFloat(f[0], 64)
			o.Load5, _ = strconv.ParseFloat(f[1], 64)
			o.Load15, _ = strconv.ParseFloat(f[2], 64)
		}
	}

	// Uptime: boot timestamp → now - boot.
	for _, l := range sec["BOOT"] {
		if m := darwinBootRe.FindStringSubmatch(l); m != nil {
			if boot, err := strconv.ParseInt(m[1], 10, 64); err == nil {
				if up := time.Since(time.Unix(boot, 0)).Seconds(); up > 0 {
					o.UptimeSeconds = up
				}
			}
		}
	}

	if len(sec["KERNEL"]) > 0 {
		o.Kernel = strings.TrimSpace(sec["KERNEL"][0])
	}

	// Process counts: "running total".
	for _, l := range sec["PROC"] {
		f := strings.Fields(l)
		if len(f) == 2 {
			o.ProcessRunning, _ = strconv.Atoi(f[0])
			o.ProcessTotal, _ = strconv.Atoi(f[1])
		}
	}

	o.Disks = parseDisks(sec["DF"])
	o.TCPStates = parseTCPStates(sec["TCP"])

	// Network: "iface rxBytes txBytes" sampled twice.
	n1 := parseDarwinNet(sec["NET1"])
	n2 := parseDarwinNet(sec["NET2"])
	if elapsedSec > 0 {
		for iface, s2 := range n2 {
			if s1, ok := n1[iface]; ok {
				if d := s2[0] - s1[0]; d > 0 {
					o.NetRxBytesPerSec += d / elapsedSec
				}
				if d := s2[1] - s1[1]; d > 0 {
					o.NetTxBytesPerSec += d / elapsedSec
				}
			}
		}
	}

	return o, nil
}

func parseDarwinNet(lines []string) map[string][2]float64 {
	m := make(map[string][2]float64)
	for _, l := range lines {
		f := strings.Fields(l)
		if len(f) != 3 {
			continue
		}
		rx, e1 := strconv.ParseFloat(f[1], 64)
		tx, e2 := strconv.ParseFloat(f[2], 64)
		if e1 == nil && e2 == nil {
			m[f[0]] = [2]float64{rx, tx}
		}
	}
	return m
}

// parseCpuTime parses ps cputime formats: "MM:SS.cc", "HH:MM:SS",
// "D-HH:MM:SS" (and bare seconds) into seconds.
func parseCpuTime(s string) float64 {
	days := 0.0
	if i := strings.Index(s, "-"); i >= 0 {
		days, _ = strconv.ParseFloat(s[:i], 64)
		s = s[i+1:]
	}
	secs := 0.0
	for _, p := range strings.Split(s, ":") {
		v, err := strconv.ParseFloat(p, 64)
		if err != nil {
			return 0
		}
		secs = secs*60 + v
	}
	return days*86400 + secs
}

func buildProcessesDarwin(out string, elapsedSec float64) []domain.MonitorProcess {
	sec := sections(out)
	if elapsedSec <= 0 {
		elapsedSec = 1
	}
	type row struct {
		cpuSeconds float64
		rssKB      float64
		name       string
	}
	first := make(map[int]row)
	for _, raw := range sec["PS1"] {
		f := strings.Fields(strings.TrimRight(raw, "\r"))
		if len(f) < 4 {
			continue
		}
		pid, err := strconv.Atoi(f[0])
		if err != nil {
			continue
		}
		rss, _ := strconv.ParseFloat(f[2], 64)
		first[pid] = row{cpuSeconds: parseCpuTime(f[1]), rssKB: rss, name: strings.Join(f[3:], " ")}
	}
	procs := make([]domain.MonitorProcess, 0, len(first))
	for _, raw := range sec["PS2"] {
		f := strings.Fields(strings.TrimRight(raw, "\r"))
		if len(f) != 2 {
			continue
		}
		pid, err := strconv.Atoi(f[0])
		if err != nil || f[1] == "-" {
			continue
		}
		r, ok := first[pid]
		if !ok {
			continue
		}
		var cpu float64
		if delta := parseCpuTime(f[1]) - r.cpuSeconds; delta > 0 {
			cpu = delta / elapsedSec * 100
		}
		procs = append(procs, domain.MonitorProcess{
			PID:         pid,
			Name:        r.name,
			CommandLine: r.name,
			CPUPercent:  round1(cpu),
			RSSKB:       uint64(r.rssKB),
		})
	}
	return procs
}

func buildPortsDarwin(out string) []domain.MonitorPort {
	type key struct {
		proto, addr string
		port        int
	}
	counts := map[key]int{}
	for _, raw := range strings.Split(out, "\n") {
		line := strings.TrimRight(raw, "\r")
		f := strings.Fields(line)
		if len(f) != 2 {
			continue
		}
		proto := "tcp"
		if strings.HasSuffix(f[0], "6") {
			proto = "tcp6"
		}
		// Local address like "*.22", "127.0.0.1.8000", "::1.9000": the port
		// follows the LAST dot.
		i := strings.LastIndex(f[1], ".")
		if i < 0 {
			continue
		}
		port, err := strconv.Atoi(f[1][i+1:])
		if err != nil {
			continue
		}
		addr := f[1][:i]
		if addr == "*" || addr == "" {
			if proto == "tcp6" {
				addr = "::"
			} else {
				addr = "0.0.0.0"
			}
		}
		counts[key{proto, addr, port}]++
	}
	out2 := make([]domain.MonitorPort, 0, len(counts))
	for k, count := range counts {
		out2 = append(out2, domain.MonitorPort{Proto: k.proto, Address: k.addr, Port: k.port, Count: count})
	}
	sort.Slice(out2, func(i, j int) bool {
		if out2[i].Port != out2[j].Port {
			return out2[i].Port < out2[j].Port
		}
		return out2[i].Address < out2[j].Address
	})
	return out2
}
