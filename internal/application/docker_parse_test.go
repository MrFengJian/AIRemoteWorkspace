package application

import (
	"strings"
	"testing"
)

func TestParseDockerContainers(t *testing.T) {
	out := `{"Command":"\"/docker-entrypoint.…\"","CreatedAt":"2026-08-20 03:12:44 +0000 UTC","ID":"a1b2c3d","Image":"nginx:latest","Labels":"com.docker.compose.project=web","Names":"web-nginx-1","Networks":"bridge","Ports":"0.0.0.0:8080->80/tcp, [::]:8080->80/tcp","RunningFor":"6 days ago","Size":"0B","State":"running","Status":"Up 6 hours (healthy)"}
{"Command":"sleep infinity","CreatedAt":"2026-08-25 10:00:00 +0000 UTC","ID":"e4f5a6b","Image":"alpine","Names":"quirky_keller","Networks":"bridge","Ports":"","RunningFor":"2 hours ago","Size":"0B","State":"exited","Status":"Exited (0) 5 minutes ago"}
WARNING: some stderr noise without braces`
	cs := parseDockerContainers(out)
	if len(cs) != 2 {
		t.Fatalf("want 2 containers, got %d", len(cs))
	}
	if cs[0].Names != "web-nginx-1" || cs[0].State != "running" || cs[0].Image != "nginx:latest" {
		t.Errorf("container[0] mismatch: %+v", cs[0])
	}
	if !strings.Contains(cs[0].Ports, "8080->80") {
		t.Errorf("ports not parsed: %q", cs[0].Ports)
	}
	if cs[1].State != "exited" {
		t.Errorf("container[1] state: %q", cs[1].State)
	}
}

func TestParseDockerStats(t *testing.T) {
	out := `{"BlockIO":"0B / 12.6kB","CPUPerc":"0.12%","Container":"a1b2c3d","ID":"a1b2c3d","MemPerc":"6.41%","MemUsage":"83.2MiB / 3.84GiB","Name":"web-nginx-1","NetIO":"1.2kB / 3.4kB","PIDs":"3"}
garbage line`
	ss := parseDockerStats(out)
	if len(ss) != 1 {
		t.Fatalf("want 1 stat row, got %d", len(ss))
	}
	s := ss[0]
	if s.ContainerID != "a1b2c3d" || s.CPUPercent != "0.12%" || s.MemUsage != "83.2MiB / 3.84GiB" {
		t.Errorf("stats mismatch: %+v", s)
	}
	if s.NetIO != "1.2kB / 3.4kB" || s.BlockIO != "0B / 12.6kB" {
		t.Errorf("io mismatch: %+v", s)
	}
}

func TestParseDockerImages(t *testing.T) {
	out := `{"Containers":"N/A","CreatedAt":"2026-08-01 08:15:09 +0000 UTC","CreatedSince":"3 weeks ago","Digest":"<none>","ID":"sha256:9c1b3e","Repository":"nginx","SharedSize":"N/A","Size":"187MB","Tag":"latest","UniqueSize":"N/A"}
{"Containers":"N/A","CreatedAt":"2026-07-11 21:03:44 +0000 UTC","CreatedSince":"6 weeks ago","Digest":"<none>","ID":"sha256:1a2b3c","Repository":"<none>","SharedSize":"N/A","Size":"7.8MB","Tag":"<none>","UniqueSize":"N/A"}`
	ims := parseDockerImages(out)
	if len(ims) != 2 {
		t.Fatalf("want 2 images, got %d", len(ims))
	}
	if ims[0].Repository != "nginx" || ims[0].Tag != "latest" || ims[0].Size != "187MB" {
		t.Errorf("image[0] mismatch: %+v", ims[0])
	}
	if ims[1].Repository != "<none>" {
		t.Errorf("dangling image lost: %+v", ims[1])
	}
}

func TestParseDockerInfo(t *testing.T) {
	ver := `{"Client":{"Version":"27.1.1","Os":"linux"},"Server":{"Components":[],"Version":"27.1.1","ApiVersion":"1.46","MinAPIVersion":"1.24","GitCommit":"cc13f95","GoVersion":"go1.22.5","Os":"linux","Arch":"amd64","KernelVersion":"5.15.0-91-generic"}}`
	info := `{"ID":"f2a1","Containers":4,"ContainersRunning":3,"ContainersPaused":0,"ContainersStopped":1,"Images":12,"Driver":"overlay2","DockerRootDir":"/var/lib/docker"}`
	got := parseDockerInfo(ver, info)
	if got.Version != "27.1.1" || got.APIVersion != "1.46" {
		t.Errorf("version fields: %+v", got)
	}
	if got.OSType != "linux" || got.Arch != "amd64" || got.KernelVersion != "5.15.0-91-generic" {
		t.Errorf("platform fields: %+v", got)
	}
	if got.ContainersRunning != 3 || got.ContainersStopped != 1 || got.Images != 12 {
		t.Errorf("counters: %+v", got)
	}
	if got.DockerRootDir != "/var/lib/docker" {
		t.Errorf("root dir: %+v", got)
	}
	// info call failed → version data still usable
	got = parseDockerInfo(ver, "")
	if got.Version != "27.1.1" || got.ContainersRunning != 0 {
		t.Errorf("version-only fallback: %+v", got)
	}
}

func TestShellQuote(t *testing.T) {
	for _, in := range []string{"{{json .}}", "web-nginx-1", "0.0.0.0:80->80/tcp"} {
		q := shellQuote(in)
		if !strings.HasPrefix(q, "'") || !strings.HasSuffix(q, "'") || !strings.Contains(q, in) {
			t.Errorf("shellQuote(%q) = %q", in, q)
		}
	}
	if got := shellQuote("it's"); got != `'it'\''s'` {
		t.Errorf("embedded quote escaping: %q", got)
	}
}

func TestParseDockerNetworks(t *testing.T) {
	out := `{"CreatedAt":"2026-08-20 03:12:44.123456789 +0000 UTC","Driver":"bridge","ID":"br-1a2b3c","IPv6":false,"Internal":false,"Labels":"","Name":"web_default","Scope":"local"}
{"CreatedAt":"2026-08-20 03:12:40 +0000 UTC","Driver":"bridge","ID":"7f2e9a4b","IPv6":true,"Internal":false,"Labels":"","Name":"bridge","Scope":"local"}
{"CreatedAt":"2026-08-20 03:12:40 +0000 UTC","Driver":"null","ID":"9c1d2e3f","IPv6":false,"Internal":true,"Labels":"","Name":"none","Scope":"local"}`
	ns := parseDockerNetworks(out)
	if len(ns) != 3 {
		t.Fatalf("want 3 networks, got %d", len(ns))
	}
	if ns[0].Name != "web_default" || ns[0].Driver != "bridge" || ns[0].Scope != "local" {
		t.Errorf("network[0] mismatch: %+v", ns[0])
	}
	if !ns[1].IPv6 {
		t.Errorf("network[1] ipv6 flag lost: %+v", ns[1])
	}
	if !ns[2].Internal || ns[2].Driver != "null" {
		t.Errorf("network[2] mismatch: %+v", ns[2])
	}
}

func TestParseDockerNetworkInspect(t *testing.T) {
	out := `{"Name":"web_default","Id":"br-1a2b3c","Created":"2026-08-20T03:12:44.5Z","Scope":"local","Driver":"bridge","EnableIPv6":false,"IPAM":{"Driver":"default","Options":null,"Config":[{"Subnet":"172.18.0.0/16","Gateway":"172.18.0.1"}]},"Internal":false,"Attachable":false,"Ingress":false,"ConfigFrom":{"Network":""},"ConfigOnly":false,"Containers":{"e5f6a7b8c9":{"Name":"web-nginx-1","EndpointID":"xyz","MacAddress":"02:42:ac:12:00:02","IPv4Address":"172.18.0.2/16","IPv6Address":""},"d1c2b3a4f5":{"Name":"web-app-1","EndpointID":"abc","MacAddress":"02:42:ac:12:00:03","IPv4Address":"172.18.0.3/16","IPv6Address":""}},"Options":{},"Labels":{"com.docker.compose.network":"web_default"}}`
	d, ok := parseDockerNetworkInspect(out)
	if !ok {
		t.Fatalf("inspect not parsed")
	}
	if d.Name != "web_default" || d.Driver != "bridge" || len(d.Subnets) != 1 {
		t.Errorf("basic fields: %+v", d)
	}
	if d.Subnets[0].Subnet != "172.18.0.0/16" || d.Subnets[0].Gateway != "172.18.0.1" {
		t.Errorf("subnet mismatch: %+v", d.Subnets[0])
	}
	if len(d.Containers) != 2 {
		t.Fatalf("want 2 attached containers, got %d", len(d.Containers))
	}
	names := map[string]bool{}
	for _, c := range d.Containers {
		names[c.Name] = true
		if !strings.HasPrefix(c.IPv4Address, "172.18.0.") {
			t.Errorf("container %s ipv4: %q", c.Name, c.IPv4Address)
		}
	}
	if !names["web-nginx-1"] || !names["web-app-1"] {
		t.Errorf("container names: %v", names)
	}
	// Unparseable output is reported, not silently empty.
	if _, ok := parseDockerNetworkInspect("not json"); ok {
		t.Errorf("garbage accepted")
	}
}

func TestCapString(t *testing.T) {
	s := strings.Repeat("x", 300000)
	got := capString(s, 256<<10)
	if len(got) >= len(s) {
		t.Fatalf("capString did not truncate")
	}
	if !strings.Contains(got, "truncated") {
		t.Errorf("missing marker")
	}
	if capString("short", 1024) != "short" {
		t.Errorf("short string modified")
	}
}
