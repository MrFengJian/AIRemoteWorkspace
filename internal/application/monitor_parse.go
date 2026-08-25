// Monitor parsing: turns raw /proc text (collected over an SSH exec channel
// by monitor_service.go's scripts) into domain DTOs. Pure string → struct so
// it can be unit-tested against captured samples.
package application

import (
	"math"
	"net"
	"sort"
	"strconv"
	"strings"

	"github.com/ai-remote/workspace/internal/domain"
)

// sections splits a collector script's output at "=NAME" marker lines into
// per-section line slices. None of the data sources emit lines starting with
// '=' at column 0, so markers are unambiguous.
func sections(out string) map[string][]string {
	m := make(map[string][]string)
	cur := ""
	for _, raw := range strings.Split(out, "\n") {
		line := strings.TrimRight(raw, "\r")
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "=") {
			cur = strings.TrimPrefix(line, "=")
			if _, ok := m[cur]; !ok {
				m[cur] = nil
			}
			continue
		}
		if cur != "" {
			m[cur] = append(m[cur], line)
		}
	}
	return m
}

// CPUStat is one sample of the aggregated /proc/stat "cpu " line.
type CPUStat struct {
	Busy  float64
	Total float64
}

func parseCPUStat(line string) (CPUStat, bool) {
	fields := strings.Fields(line)
	if len(fields) < 5 || fields[0] != "cpu" {
		return CPUStat{}, false
	}
	vals := make([]float64, 0, len(fields)-1)
	for _, s := range fields[1:] {
		v, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return CPUStat{}, false
		}
		vals = append(vals, v)
	}
	var total float64
	for _, v := range vals {
		total += v
	}
	idle := vals[3]
	iowait := 0.0
	if len(vals) > 4 {
		iowait = vals[4]
	}
	return CPUStat{Busy: total - idle - iowait, Total: total}, true
}

func parseMemKB(lines []string) map[string]uint64 {
	m := make(map[string]uint64, 4)
	for _, l := range lines {
		fields := strings.Fields(l)
		if len(fields) < 2 {
			continue
		}
		v, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			continue
		}
		m[strings.TrimSuffix(fields[0], ":")] = v
	}
	return m
}

// parseNetDev maps iface → {rxBytes, txBytes} from /proc/net/dev lines
// ("lo" excluded — the overview shows external traffic).
func parseNetDev(lines []string) map[string][2]float64 {
	m := make(map[string][2]float64)
	for _, l := range lines {
		i := strings.Index(l, ":")
		if i < 0 {
			continue
		}
		iface := strings.TrimSpace(l[:i])
		if iface == "lo" {
			continue
		}
		fields := strings.Fields(l[i+1:])
		if len(fields) < 9 {
			continue
		}
		rx, e1 := strconv.ParseFloat(fields[0], 64)
		tx, e2 := strconv.ParseFloat(fields[8], 64)
		if e1 == nil && e2 == nil {
			m[iface] = [2]float64{rx, tx}
		}
	}
	return m
}

// pseudoDevices are kernel/virtual filesystems that df reports but that are
// meaningless in a usage list.
var pseudoDevices = []string{
	"tmpfs", "devtmpfs", "udev", "overlay", "squashfs", "shm", "proc", "sysfs",
	"devpts", "cgroup", "cgmfs", "mqueue", "hugetlbfs", "efivarfs", "autofs",
	"nsfs", "fusectl", "binfmt_misc", "configfs", "debugfs", "tracefs",
	"pstore", "securityfs", "bpf", "dax", "ramfs", "erofs", "iso9660", "fuse.",
}

func parseDisks(lines []string) []domain.MonitorDiskUsage {
	var out []domain.MonitorDiskUsage
	for _, l := range lines {
		fields := strings.Fields(l)
		if len(fields) < 6 || fields[0] == "Filesystem" {
			continue
		}
		dev := fields[0]
		if strings.HasPrefix(dev, "/dev/loop") {
			continue // snap/flatpak loop mounts
		}
		pseudo := false
		for _, p := range pseudoDevices {
			if strings.HasPrefix(dev, p) {
				pseudo = true
				break
			}
		}
		if pseudo {
			continue
		}
		total, e1 := strconv.ParseUint(fields[1], 10, 64)
		used, e2 := strconv.ParseUint(fields[2], 10, 64)
		if e1 != nil || e2 != nil || total == 0 {
			continue
		}
		out = append(out, domain.MonitorDiskUsage{
			Device:      dev,
			Mount:       strings.Join(fields[5:], " "),
			TotalKB:     total,
			UsedKB:      used,
			UsedPercent: math.Round(float64(used)/float64(total)*1000) / 10,
		})
	}
	return out
}

var tcpStateNames = map[string]string{
	"01": "ESTABLISHED", "02": "SYN_SENT", "03": "SYN_RECV", "04": "FIN_WAIT1",
	"05": "FIN_WAIT2", "06": "TIME_WAIT", "07": "CLOSE", "08": "CLOSE_WAIT",
	"09": "LAST_ACK", "0A": "LISTEN", "0B": "CLOSING", "0C": "NEW_SYN_RECV",
}

func parseTCPStates(lines []string) []domain.MonitorTCPState {
	counts := map[string]int{}
	for _, l := range lines {
		fields := strings.Fields(l)
		if len(fields) != 2 {
			continue
		}
		n, err := strconv.Atoi(fields[1])
		if err != nil {
			continue
		}
		// Linux /proc reports hex state codes ("0A 8"); macOS netstat reports
		// names ("LISTEN 8"). Accept both.
		name, ok := tcpStateNames[fields[0]]
		if !ok {
			name = fields[0]
		}
		counts[name] += n
	}
	out := make([]domain.MonitorTCPState, 0, len(counts))
	for state, count := range counts {
		out = append(out, domain.MonitorTCPState{State: state, Count: count})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Count > out[j].Count })
	return out
}

func round1(v float64) float64 { return math.Round(v*10) / 10 }

func firstLine(lines []string) string {
	if len(lines) > 0 {
		return lines[0]
	}
	return ""
}

func firstStat(lines []string) (CPUStat, bool) {
	for _, l := range lines {
		if s, ok := parseCPUStat(l); ok {
			return s, true
		}
	}
	return CPUStat{}, false
}

// buildOverview assembles the overview DTO from a completed overview script
// run. elapsed is the measured wall time of the exec (the script samples
// twice with a 1s sleep in between), used for network rate math.
func buildOverview(out string, elapsedSec float64) (domain.MonitorOverview, error) {
	sec := sections(out)
	if len(sec) == 0 {
		return domain.MonitorOverview{}, errMonitorEmpty
	}
	o := domain.MonitorOverview{}

	for _, l := range sec["CPUINFO"] {
		if strings.HasPrefix(l, "cores ") {
			o.CPUCores, _ = strconv.Atoi(strings.TrimPrefix(l, "cores "))
		} else if o.CPUModel == "" {
			if i := strings.Index(l, ": "); i >= 0 {
				o.CPUModel = l[i+2:]
			}
		}
	}

	if s1, ok := firstStat(sec["STAT1"]); ok {
		if s2, ok2 := firstStat(sec["STAT2"]); ok2 && s2.Total > s1.Total {
			o.CPUPercent = round1(math.Max(0, math.Min(100, (s2.Busy-s1.Busy)/(s2.Total-s1.Total)*100)))
		}
	}

	mem := parseMemKB(sec["MEM"])
	o.MemTotalKB = mem["MemTotal"]
	if total, avail := mem["MemTotal"], mem["MemAvailable"]; total > avail {
		o.MemUsedKB = total - avail
	}
	if o.MemTotalKB > 0 {
		o.MemUsedPercent = round1(float64(o.MemUsedKB) / float64(o.MemTotalKB) * 100)
	}
	if swapTotal, swapFree := mem["SwapTotal"], mem["SwapFree"]; swapTotal > swapFree {
		o.SwapTotalKB, o.SwapUsedKB = swapTotal, swapTotal-swapFree
	}

	if len(sec["LOAD"]) > 0 {
		f := strings.Fields(sec["LOAD"][0])
		if len(f) >= 4 {
			o.Load1, _ = strconv.ParseFloat(f[0], 64)
			o.Load5, _ = strconv.ParseFloat(f[1], 64)
			o.Load15, _ = strconv.ParseFloat(f[2], 64)
			if rt := strings.Split(f[3], "/"); len(rt) == 2 {
				o.ProcessRunning, _ = strconv.Atoi(rt[0])
				o.ProcessTotal, _ = strconv.Atoi(rt[1])
			}
		}
	}

	if len(sec["UP"]) > 0 {
		f := strings.Fields(sec["UP"][0])
		if len(f) > 0 {
			o.UptimeSeconds, _ = strconv.ParseFloat(f[0], 64)
		}
	}

	if len(sec["KERNEL"]) > 0 {
		o.Kernel = sec["KERNEL"][0]
	}

	o.Disks = parseDisks(sec["DF"])
	o.TCPStates = parseTCPStates(sec["TCP"])

	n1, n2 := parseNetDev(sec["NET1"]), parseNetDev(sec["NET2"])
	if elapsedSec > 0 {
		for iface, s2 := range n2 {
			if s1, ok := n1[iface]; ok {
				o.NetRxBytesPerSec += math.Max(0, s2[0]-s1[0]) / elapsedSec
				o.NetTxBytesPerSec += math.Max(0, s2[1]-s1[1]) / elapsedSec
			}
		}
	}

	return o, nil
}

type procSample struct {
	ut, st, rssPages float64
	name, cmd        string
}

// buildProcesses merges the script's two /proc/[pid]/stat samples into
// process rows with live CPU usage. CLK_TCK / PAGESIZE come from the script
// output itself (getconf, with busybox fallbacks). rssPages × pageSize/1024
// gives RSS KB.
func buildProcesses(out string, elapsedSec float64) []domain.MonitorProcess {
	sec := sections(out)
	clkTCK, pageSize := 100, 4096
	if v, err := strconv.Atoi(firstLine(sec["CLKTCK"])); err == nil && v > 0 {
		clkTCK = v
	}
	if v, err := strconv.Atoi(firstLine(sec["PAGESIZE"])); err == nil && v > 0 {
		pageSize = v
	}
	hz := float64(clkTCK)
	if elapsedSec <= 0 {
		elapsedSec = 1
	}

	parseSample := func(lines []string, withMeta bool,
		first map[string]procSample, second map[string][2]float64) {
		for _, raw := range lines {
			line := strings.TrimRight(raw, "\r")
			f := strings.Split(line, "\t")
			if withMeta {
				if len(f) < 6 {
					continue
				}
				ut, e1 := strconv.ParseFloat(f[2], 64)
				st, e2 := strconv.ParseFloat(f[3], 64)
				rss, e3 := strconv.ParseFloat(f[4], 64)
				if e1 != nil || e2 != nil || e3 != nil {
					continue
				}
				cmd := ""
				if len(f) == 7 {
					cmd = f[6]
				}
				first[f[1]] = procSample{ut: ut, st: st, rssPages: rss, name: f[5], cmd: cmd}
			} else {
				if len(f) != 4 {
					continue
				}
				ut, e1 := strconv.ParseFloat(f[2], 64)
				st, e2 := strconv.ParseFloat(f[3], 64)
				if e1 != nil || e2 != nil {
					continue
				}
				second[f[1]] = [2]float64{ut, st}
			}
		}
	}

	first := make(map[string]procSample)
	second := make(map[string][2]float64)
	parseSample(sec["PS1"], true, first, second)
	parseSample(sec["PS2"], false, first, second)

	procs := make([]domain.MonitorProcess, 0, len(first))
	for pid, s := range first {
		s2, ok := second[pid]
		if !ok {
			continue // exited between samples
		}
		ticks := s2[0] + s2[1] - s.ut - s.st
		cpu := math.Max(0, ticks/(hz*elapsedSec)*100)
		p, err := strconv.Atoi(pid)
		if err != nil {
			continue
		}
		procs = append(procs, domain.MonitorProcess{
			PID:         p,
			Name:        s.name,
			CommandLine: s.cmd,
			CPUPercent:  round1(cpu),
			RSSKB:       uint64(s.rssPages * float64(pageSize) / 1024),
		})
	}
	return procs
}

// buildPorts parses the ports script output ("FILENAME localHex:portHex"
// per listening socket) into deduplicated bind rows sorted by port.
func buildPorts(out string) []domain.MonitorPort {
	type key struct {
		proto, addr string
		port        int
	}
	counts := map[key]int{}
	for _, raw := range strings.Split(out, "\n") {
		line := strings.TrimRight(raw, "\r")
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		proto := "tcp"
		if strings.HasSuffix(fields[0], "6") {
			proto = "tcp6"
		}
		ap := strings.Split(fields[1], ":")
		if len(ap) != 2 {
			continue
		}
		port, err := strconv.ParseInt(ap[1], 16, 32)
		if err != nil {
			continue
		}
		counts[key{proto, decodeHexAddr(ap[0], proto == "tcp6"), int(port)}]++
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

// decodeHexAddr turns a /proc/net/tcp* local-address hex field (little-endian
// per 4-byte group) into a readable IP string.
func decodeHexAddr(hex string, v6 bool) string {
	if !v6 {
		if len(hex) != 8 {
			return "0.0.0.0"
		}
		var b [4]byte
		for i := 0; i < 4; i++ {
			v, err := strconv.ParseUint(hex[i*2:i*2+2], 16, 8)
			if err != nil {
				return "0.0.0.0"
			}
			b[3-i] = byte(v)
		}
		return net.IP(b[:]).String()
	}
	if len(hex) != 32 {
		return "::"
	}
	b := make([]byte, 0, 16)
	for i := 0; i < 32; i += 8 {
		for j := 3; j >= 0; j-- {
			v, err := strconv.ParseUint(hex[i+j*2:i+j*2+2], 16, 8)
			if err != nil {
				return "::"
			}
			b = append(b, byte(v))
		}
	}
	return net.IP(b).String()
}

type monitorError string

func (e monitorError) Error() string { return string(e) }

const errMonitorEmpty = monitorError("monitor: collector returned no data")
