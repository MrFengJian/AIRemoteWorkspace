package application

import (
	"math"
	"testing"
)

const overviewSample = `=CPUINFO
model name	: Intel(R) Xeon(R) CPU E5-2682 v4 @ 2.50GHz
cores 4
=STAT1
cpu  100 0 50 800 10 0 0 0 0 0
=NET1
Inter-|   Receive                                                |  Transmit
 face |bytes    packets errs drop fifo frame compressed multicast|bytes    packets errs drop fifo colls carrier compressed
  eth0: 1000 10 0 0 0 0 0 0        2000 20 0 0 0 0 0 0
    lo: 500 5 0 0 0 0 0 0        500 5 0 0 0 0 0 0
sleep 1 marker ignored
=STAT2
cpu  200 0 100 900 10 0 0 0 0 0
=NET2
Inter-|   Receive                                                |  Transmit
 face |bytes    packets errs drop fifo frame compressed multicast|bytes    packets errs drop fifo colls carrier compressed
  eth0: 2500 25 0 0 0 0 0 0        3000 30 0 0 0 0 0 0
    lo: 700 7 0 0 0 0 0 0        700 7 0 0 0 0 0 0
=MEM
MemTotal:       16384000 kB
MemAvailable:    8192000 kB
SwapTotal:       2048000 kB
SwapFree:        1024000 kB
=LOAD
0.15 0.10 0.05 1/389 12345
=UP
98765.42 123456.78
=KERNEL
5.15.0-91-generic
=DF
Filesystem     1024-blocks      Used Available Capacity Mounted on
/dev/vda1         41152812  12345678  26664722      32% /
tmpfs              2048000         0   2048000       0% /dev/shm
/dev/vdb1        103080888  51540444  46304000      50% /data
=TCP
01 42
0A 8
06 15
`

func TestBuildOverview(t *testing.T) {
	o, err := buildOverview(overviewSample, 1.5)
	if err != nil {
		t.Fatalf("buildOverview: %v", err)
	}
	// CPU: busy1=150, total1=960; busy2=300, total2=1210 → 150/250 = 60%.
	if o.CPUPercent != 60 {
		t.Errorf("CPUPercent = %v, want 60", o.CPUPercent)
	}
	if o.CPUModel != "Intel(R) Xeon(R) CPU E5-2682 v4 @ 2.50GHz" {
		t.Errorf("CPUModel = %q", o.CPUModel)
	}
	if o.CPUCores != 4 {
		t.Errorf("CPUCores = %v, want 4", o.CPUCores)
	}
	// Memory: used = total - available = 8192000 (50%).
	if o.MemUsedKB != 8192000 || o.MemTotalKB != 16384000 || o.MemUsedPercent != 50 {
		t.Errorf("mem = %v/%v %v%%", o.MemUsedKB, o.MemTotalKB, o.MemUsedPercent)
	}
	if o.SwapTotalKB != 2048000 || o.SwapUsedKB != 1024000 {
		t.Errorf("swap = %v/%v", o.SwapUsedKB, o.SwapTotalKB)
	}
	if o.Load1 != 0.15 || o.ProcessRunning != 1 || o.ProcessTotal != 389 {
		t.Errorf("load/proc = %v %v/%v", o.Load1, o.ProcessRunning, o.ProcessTotal)
	}
	if o.UptimeSeconds != 98765.42 {
		t.Errorf("uptime = %v", o.UptimeSeconds)
	}
	if o.Kernel != "5.15.0-91-generic" {
		t.Errorf("kernel = %q", o.Kernel)
	}
	// Network: eth0 only (lo excluded); rx Δ=1500/1.5=1000, tx Δ=1000/1.5≈667.
	if math.Abs(o.NetRxBytesPerSec-1000) > 0.01 || math.Abs(o.NetTxBytesPerSec-1000/1.5) > 0.01 {
		t.Errorf("net rates = %v / %v", o.NetRxBytesPerSec, o.NetTxBytesPerSec)
	}
	// Disks: tmpfs filtered, /data 50% kept.
	if len(o.Disks) != 2 {
		t.Fatalf("disks = %v, want 2 entries", o.Disks)
	}
	if o.Disks[0].Mount != "/" || o.Disks[1].UsedPercent != 50 {
		t.Errorf("disks = %+v", o.Disks)
	}
	// TCP histogram sorted by count desc.
	if len(o.TCPStates) != 3 || o.TCPStates[0].State != "ESTABLISHED" || o.TCPStates[0].Count != 42 {
		t.Errorf("tcpStates = %+v", o.TCPStates)
	}
}

func TestBuildOverviewEmpty(t *testing.T) {
	if _, err := buildOverview("", 1); err == nil {
		t.Fatal("expected error on empty output")
	}
}

const processSample = `=PAGESIZE
4096
=CLKTCK
100
=PS1
P	1	100	50	30	(systemd)	/sbin/init splash
P	42	200	100	250	(bash)	bash
P	77	0	0	1000	(nginx:)	nginx: master process nginx
=PS2
Q	1	100	50
Q	42	400	200
Q	78	0	0
`

func TestBuildProcesses(t *testing.T) {
	procs := buildProcesses(processSample, 2.0)
	if len(procs) != 2 { // pid 77 exited between samples
		t.Fatalf("procs = %+v, want 2", procs)
	}
	byPID := map[int]struct {
		cpu float64
		rss uint64
	}{}
	for _, p := range procs {
		byPID[p.PID] = struct {
			cpu float64
			rss uint64
		}{p.CPUPercent, p.RSSKB}
	}
	// pid 42: Δticks = (200+400)-(200+100) = 300; 300/(100*2) = 1.5 → 150%.
	if got := byPID[42]; got.cpu != 150 || got.rss != 250*4096/1024 {
		t.Errorf("pid42 = %+v", byPID[42])
	}
	// pid 1: no delta → 0%.
	if got := byPID[1]; got.cpu != 0 || got.rss != 30*4096/1024 {
		t.Errorf("pid1 = %+v", byPID[1])
	}
}

const portsSample = `/proc/net/tcp 0100007F:0019
/proc/net/tcp 00000000:0050
/proc/net/tcp 00000000:0050
/proc/net/tcp6 00000000000000000000000000000000:0016
`

func TestBuildPorts(t *testing.T) {
	ports := buildPorts(portsSample)
	if len(ports) != 3 {
		t.Fatalf("ports = %+v, want 3", ports)
	}
	// Sorted by port asc: 22 (tcp6 ::), 25 (tcp 127.0.0.1), 80 (0.0.0.0 ×2).
	if ports[0].Port != 22 || ports[0].Address != "::" || ports[0].Proto != "tcp6" {
		t.Errorf("port0 = %+v", ports[0])
	}
	// 0100007F is little-endian → 127.0.0.1.
	if ports[1].Port != 25 || ports[1].Address != "127.0.0.1" || ports[1].Count != 1 {
		t.Errorf("port1 = %+v", ports[1])
	}
	// Port 80: two sockets on 0.0.0.0 → one row, count 2.
	if ports[2].Port != 80 || ports[2].Address != "0.0.0.0" || ports[2].Count != 2 {
		t.Errorf("port2 = %+v", ports[2])
	}
}
