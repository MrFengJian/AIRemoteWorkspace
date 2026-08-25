package application

import (
	"testing"
)

const darwinOverviewSample = `=CPUINFO
Apple M2 Pro
cores 10
=STATIDLE
CPU usage: 4.20% user, 2.31% sys, 93.49% idle, 0.00% iowait
=NET1
en0 1000 2000
en1 500 100
=NET2
en0 2500 3000
en1 500 100
=MEM
17179869184
Mach Virtual Memory Statistics: (page size of 16384 bytes)
Pages free:                               100000.
Pages inactive:                           200000.
Pages speculative:                         50000.
Pages active:                             300000.
Pages wired down:                         150000.
=SWAP
total = 2048.00M  used = 100.00M  free = 1948.00M
=LOAD
{ 1.92 1.87 1.90 }
=BOOT
{ sec = 1700000000, usec = 0 } Thu Nov 14 21:06:40 2024
=KERNEL
23.4.0
=DF
Filesystem     1024-blocks      Used Available Capacity Mounted on
/dev/disk3s1s1   486513540  22900000 199999999      11% /
map auto_home           0         0         0     100% /System/Volumes/Data/home
/dev/disk3s5     486513540 100000000 122663540      45% /System/Volumes/Data
=TCP
ESTABLISHED 42
LISTEN 8
=PROC
3 402
`

func TestBuildOverviewDarwin(t *testing.T) {
	o, err := buildOverviewDarwin(darwinOverviewSample, 1.5)
	if err != nil {
		t.Fatalf("buildOverviewDarwin: %v", err)
	}
	if o.CPUPercent != 6.5 {
		t.Errorf("CPUPercent = %v, want 6.5 (100-93.49)", o.CPUPercent)
	}
	if o.CPUModel != "Apple M2 Pro" || o.CPUCores != 10 {
		t.Errorf("cpu info = %q / %v", o.CPUModel, o.CPUCores)
	}
	// memsize 17179869184 B = 16777216 KB; available = (100000+200000+50000)*16384/1024 = 5600000 KB.
	if o.MemTotalKB != 16777216 {
		t.Errorf("MemTotalKB = %v", o.MemTotalKB)
	}
	if o.MemUsedKB != 16777216-5600000 {
		t.Errorf("MemUsedKB = %v, want %v", o.MemUsedKB, 16777216-5600000)
	}
	if o.SwapTotalKB != 2048*1024 || o.SwapUsedKB != 100*1024 {
		t.Errorf("swap = %v/%v", o.SwapUsedKB, o.SwapTotalKB)
	}
	if o.Load1 != 1.92 || o.ProcessRunning != 3 || o.ProcessTotal != 402 {
		t.Errorf("load/proc = %v %v/%v", o.Load1, o.ProcessRunning, o.ProcessTotal)
	}
	if o.UptimeSeconds <= 0 {
		t.Errorf("uptime = %v, want > 0", o.UptimeSeconds)
	}
	// en0: rx Δ=1500/1.5=1000; tx Δ=1000/1.5≈667; en1 unchanged.
	if o.NetRxBytesPerSec != 1000 || o.NetTxBytesPerSec != 1000/1.5 {
		t.Errorf("net = %v / %v", o.NetRxBytesPerSec, o.NetTxBytesPerSec)
	}
	// map auto_home filtered.
	if len(o.Disks) != 2 {
		t.Fatalf("disks = %+v, want 2", o.Disks)
	}
	if o.TCPStates[0].State != "ESTABLISHED" || o.TCPStates[0].Count != 42 {
		t.Errorf("tcp = %+v", o.TCPStates)
	}
}

const darwinProcessSample = `=PS1
1 10:00.01 100 launchd
42 0:02.00 20480 /bin/bash -l
=PS2
1 10:00.01
42 0:04.50
`

func TestBuildProcessesDarwin(t *testing.T) {
	procs := buildProcessesDarwin(darwinProcessSample, 2.0)
	if len(procs) != 2 {
		t.Fatalf("procs = %+v", procs)
	}
	// pid 42: Δcputime = 2.5s over 2s → 125%.
	if procs[1].PID != 42 || procs[1].CPUPercent != 125 || procs[1].RSSKB != 20480 {
		t.Errorf("pid42 = %+v", procs[1])
	}
	if procs[0].PID != 1 || procs[0].CPUPercent != 0 {
		t.Errorf("pid1 = %+v", procs[0])
	}
}

const darwinPortsSample = `tcp4 *.22
tcp4 127.0.0.1.8000
tcp6 *.8080
tcp6 *.8080
`

func TestBuildPortsDarwin(t *testing.T) {
	ports := buildPortsDarwin(darwinPortsSample)
	if len(ports) != 3 {
		t.Fatalf("ports = %+v", ports)
	}
	if ports[0].Port != 22 || ports[0].Address != "0.0.0.0" || ports[0].Proto != "tcp" {
		t.Errorf("port0 = %+v", ports[0])
	}
	if ports[1].Port != 8000 || ports[1].Address != "127.0.0.1" {
		t.Errorf("port1 = %+v", ports[1])
	}
	if ports[2].Port != 8080 || ports[2].Address != "::" || ports[2].Proto != "tcp6" || ports[2].Count != 2 {
		t.Errorf("port2 = %+v", ports[2])
	}
}

func TestParseCpuTime(t *testing.T) {
	cases := []struct {
		in   string
		want float64
	}{
		{"0:02.50", 2.5},
		{"1:30:00", 5400},
		{"1-02:03:04", 93784},
		{"42.75", 42.75},
	}
	for _, c := range cases {
		if got := parseCpuTime(c.in); got != c.want {
			t.Errorf("parseCpuTime(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}
