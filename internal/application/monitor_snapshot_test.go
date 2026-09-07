package application

import (
	"strings"
	"testing"
	"time"

	"github.com/ai-remote/workspace/internal/domain"
)

func TestBuildSnapshotText(t *testing.T) {
	now := time.Date(2026, 9, 7, 10, 0, 0, 0, time.UTC)
	o := domain.MonitorOverview{
		CPUPercent:    83.2,
		CPUModel:      "Intel Xeon",
		CPUCores:      8,
		Kernel:        "5.15.0",
		Load1:         4.2,
		Load5:         3.1,
		Load15:        2.0,
		UptimeSeconds: 3 * 86400,
		MemTotalKB:    8 * 1024 * 1024,
		MemUsedKB:     5 * 1024 * 1024,
		MemUsedPercent: 62.5,
		SwapTotalKB:   2 * 1024 * 1024,
		SwapUsedKB:    100 * 1024,
		Disks: []domain.MonitorDiskUsage{
			{Device: "/dev/sda1", Mount: "/", TotalKB: 40 * 1024 * 1024, UsedKB: 38 * 1024 * 1024, UsedPercent: 95},
			{Device: "tmpfs", Mount: "/tmp", TotalKB: 1024, UsedKB: 1, UsedPercent: 0.1},
		},
	}
	procs := []domain.MonitorProcess{
		{PID: 101, Name: "java", CommandLine: "/usr/bin/java -Xmx4g app.jar", CPUPercent: 210.5, RSSKB: 4 * 1024 * 1024},
		{PID: 202, Name: "nginx", CommandLine: "nginx: worker process", CPUPercent: 5.0, RSSKB: 50 * 1024},
	}
	ports := []domain.MonitorPort{
		{Proto: "tcp", Address: "0.0.0.0", Port: 22},
		{Proto: "tcp", Address: "0.0.0.0", Port: 443},
	}
	errLogs := "=FAILED\nnginx.service loaded failed failed\n\n=JOURNAL\nSep 07 09:59:01 host systemd[1]: nginx.service failed\n\n=DMESG\n[Mon Sep  7 09:00] Out of memory: Killed process 101\n\n"

	out := buildSnapshotText(now, o, procs, true, ports, errLogs)

	for _, want := range []string{
		"CPU: 83.2%", "load: 4.20 3.10 2.00",
		"Memory: 5.0G of 8.0G (62.5%)", "swap: 100.0M of 2.0G",
		"/dev/sda1 on /: 95%", // tmpfs filtered out
		"101 210.5% cpu 4.0G mem", "/usr/bin/java",
		"Listening ports: *:22, *:443",
		"nginx.service loaded failed",
		"Out of memory: Killed process 101",
		"Uptime: 3d0h",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("snapshot missing %q:\n%s", want, out)
		}
	}

	// Sections that failed collection are simply absent, never error markers.
	minimal := buildSnapshotText(now, domain.MonitorOverview{CPUPercent: 1}, nil, false, nil, "")
	if !strings.Contains(minimal, "CPU: 1.0%") {
		t.Fatalf("minimal snapshot broken:\n%s", minimal)
	}
	if strings.Contains(minimal, "Top processes") || strings.Contains(minimal, "Recent errors") {
		t.Fatalf("empty sections should be omitted:\n%s", minimal)
	}
}

func TestParseErrorSections(t *testing.T) {
	if f, j, d := parseErrorSections(""); f != "" || j != "" || d != "" {
		t.Fatalf("empty raw must yield empty sections: %q %q %q", f, j, d)
	}
	raw := "=FAILED\na.service\n=JOURNAL\nline1\nline2\n=DMESG\ncall trace"
	f, j, d := parseErrorSections(raw)
	if f != "a.service" || j != "line1\nline2" || d != "call trace" {
		t.Fatalf("parse mismatch: %q | %q | %q", f, j, d)
	}
}
