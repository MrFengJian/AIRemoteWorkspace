package application

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/ai-remote/workspace/internal/domain"
)

// MonitorCacheTTL is the minimum interval between two collections of the
// same host + section. The monitor panel (Wails) and the MCP host_stats
// tool read through the same cache, so both consumers — and every terminal
// tab of one host — share one collection instead of each exec'ing its own.
const MonitorCacheTTL = 10 * time.Second

// MonitorService collects host metrics over an existing SSH session's exec
// channel. Scripts read the remote /proc filesystem (plus df) — nothing to
// install on the host; every command used (cat, grep, awk, df, getconf,
// sleep, uname) ships with mainstream distros and busybox. CPU usage and
// network rates come from two samples taken 1s apart; the measured exec wall
// time is the rate denominator.
//
// Results are cached per host (all sessions of one host share a scope) for
// MonitorCacheTTL; collection errors are never cached.
type MonitorService struct {
	connect ConnectionManager

	mu       sync.Mutex
	cache    map[string]monitorCacheEntry
	cacheTTL time.Duration
	now      func() time.Time // overridable in tests
}

// NewMonitorService wires a MonitorService to the connection manager.
func NewMonitorService(connect ConnectionManager) *MonitorService {
	return &MonitorService{
		connect:  connect,
		cache:    map[string]monitorCacheEntry{},
		cacheTTL: MonitorCacheTTL,
		now:      time.Now,
	}
}

// Scripts emit "=SECTION" marker lines; monitor_parse.go consumes them.
// Parsing is marker-driven, so section order is cosmetic.

// overviewScript gathers every overview metric in one exec round-trip.
const overviewScript = `echo =CPUINFO
grep -m1 '^model name' /proc/cpuinfo 2>/dev/null || grep -m1 '^cpu model' /proc/cpuinfo 2>/dev/null || grep -m1 '^model' /proc/cpuinfo 2>/dev/null
echo "cores $(grep -c ^processor /proc/cpuinfo 2>/dev/null || echo 1)"
echo =STAT1
grep '^cpu ' /proc/stat
echo =NET1
cat /proc/net/dev
sleep 1
echo =STAT2
grep '^cpu ' /proc/stat
echo =NET2
cat /proc/net/dev
echo =MEM
grep -E '^(MemTotal|MemAvailable|SwapTotal|SwapFree):' /proc/meminfo
echo =LOAD
cat /proc/loadavg
echo =UP
cat /proc/uptime
echo =KERNEL
uname -r 2>/dev/null
echo =DF
df -P -k 2>/dev/null
echo =TCP
awk 'NR>1 {s[$4]++} END {for (k in s) print k, s[k]}' /proc/net/tcp /proc/net/tcp6 2>/dev/null`

// processScript samples /proc/[pid]/stat twice via one awk pass per sample
// (fields after the first ")" are re-split so comms containing spaces don't
// shift columns). cmdline comes from /proc/<pid>/cmdline with NULs → spaces.
const processScript = `echo =PAGESIZE
getconf PAGESIZE 2>/dev/null || echo 4096
echo =CLKTCK
getconf CLK_TCK 2>/dev/null || echo 100
echo =PS1
awk '{
  line = $0; c = index(line, ")"); if (c == 0) next
  rest = substr(line, c + 2)
  n = split(rest, f, " "); if (n < 22) next
  comm = substr(line, 2, c - 2)
  fn = FILENAME; sub(/^\/proc\//, "", fn); sub(/\/stat$/, "", fn)
  cmd = ""; cf = "/proc/" fn "/cmdline"
  if ((getline cl < cf) > 0) { gsub(/\t/, " ", cl); gsub(/\0/, " ", cl); cmd = substr(cl, 1, 200); close(cf) }
  printf "P\t%s\t%s\t%s\t%s\t%s\t%s\n", fn, f[12], f[13], f[22], comm, cmd
}' /proc/[0-9]*/stat 2>/dev/null
sleep 1
echo =PS2
awk '{
  line = $0; c = index(line, ")"); if (c == 0) next
  rest = substr(line, c + 2)
  n = split(rest, f, " "); if (n < 13) next
  fn = FILENAME; sub(/^\/proc\//, "", fn); sub(/\/stat$/, "", fn)
  printf "Q\t%s\t%s\t%s\n", fn, f[12], f[13]
}' /proc/[0-9]*/stat 2>/dev/null`

// portScript lists listening TCP sockets (st == 0A) from both families.
const portScript = `awk '$4 == "0A" {print FILENAME, $2}' /proc/net/tcp /proc/net/tcp6 2>/dev/null`

func (s *MonitorService) collect(ctx context.Context, sessionID, script string) (string, float64, error) {
	if s.connect == nil {
		return "", 0, errors.New("monitor: no connection manager")
	}
	start := time.Now()
	out, err := s.connect.ExecInSessionCtx(ctx, sessionID, script)
	elapsed := time.Since(start).Seconds()
	if err != nil {
		return "", 0, fmt.Errorf("monitor: exec: %w", err)
	}
	return out, elapsed, nil
}

// GetOverview collects the CPU / memory / disk / network / load / process /
// TCP-state snapshot for the host behind sessionID (cached, see
// MonitorCacheTTL).
func (s *MonitorService) GetOverview(ctx context.Context, sessionID string) (domain.MonitorOverview, error) {
	scope := s.scopeFor(sessionID)
	if v, ok := monitorCacheGet[domain.MonitorOverview](s, scope, "overview"); ok {
		return v, nil
	}
	var o domain.MonitorOverview
	var err error
	if isLocalSessionID(sessionID) {
		o, err = s.localOverview(ctx)
	} else {
		var out string
		var elapsed float64
		if out, elapsed, err = s.collect(ctx, sessionID, overviewScript); err == nil {
			o, err = buildOverview(out, elapsed)
		}
	}
	if err != nil {
		return domain.MonitorOverview{}, err
	}
	monitorCachePut(s, scope, "overview", o)
	return o, nil
}

// GetProcesses collects the live process list (sampled CPU%, RSS KB, name,
// command line) for the host behind sessionID (cached).
func (s *MonitorService) GetProcesses(ctx context.Context, sessionID string) ([]domain.MonitorProcess, error) {
	scope := s.scopeFor(sessionID)
	if v, ok := monitorCacheGet[[]domain.MonitorProcess](s, scope, "processes"); ok {
		return v, nil
	}
	var procs []domain.MonitorProcess
	var err error
	if isLocalSessionID(sessionID) {
		procs, err = s.localProcesses(ctx)
	} else {
		var out string
		var elapsed float64
		if out, elapsed, err = s.collect(ctx, sessionID, processScript); err == nil {
			procs = buildProcesses(out, elapsed)
		}
	}
	if err != nil {
		return nil, err
	}
	monitorCachePut(s, scope, "processes", procs)
	return procs, nil
}

// GetPorts collects the listening-socket list (TCP + TCP6, deduplicated per
// identical bind) for the host behind sessionID (cached).
func (s *MonitorService) GetPorts(ctx context.Context, sessionID string) ([]domain.MonitorPort, error) {
	scope := s.scopeFor(sessionID)
	if v, ok := monitorCacheGet[[]domain.MonitorPort](s, scope, "ports"); ok {
		return v, nil
	}
	var ports []domain.MonitorPort
	var err error
	if isLocalSessionID(sessionID) {
		ports, err = s.localPorts(ctx)
	} else {
		var out string
		if out, _, err = s.collect(ctx, sessionID, portScript); err == nil {
			ports = buildPorts(out)
		}
	}
	if err != nil {
		return nil, err
	}
	monitorCachePut(s, scope, "ports", ports)
	return ports, nil
}
