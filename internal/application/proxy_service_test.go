package application

import (
	"errors"
	"testing"

	"github.com/ai-remote/workspace/internal/domain"
)

// fakeHostSource fakes the slice of HostService the ProxyService needs.
type fakeHostSource struct {
	hosts map[string]domain.Host
	creds map[string]domain.Credentials
}

func (f *fakeHostSource) Get(id string) (domain.Host, error) {
	h, ok := f.hosts[id]
	if !ok {
		return domain.Host{}, errors.New("host not found")
	}
	return h, nil
}

func (f *fakeHostSource) ResolveCredentials(host domain.Host, _ domain.Credentials) (domain.Credentials, error) {
	if c, ok := f.creds[host.ID]; ok {
		return c, nil
	}
	return domain.Credentials{}, nil
}

func newFakeProxyHosts() *fakeHostSource {
	return &fakeHostSource{
		hosts: map[string]domain.Host{
			"target": {ID: "target", Name: "inner", Host: "10.0.0.1", Port: 22,
				Proxy: &domain.ProxyConfig{Kind: domain.ProxyJump, HostID: "dmz"}},
			"dmz": {ID: "dmz", Name: "dmz", Host: "203.0.113.2", Port: 2222,
				Proxy: &domain.ProxyConfig{Kind: domain.ProxyJump, HostID: "edge"}},
			"edge": {ID: "edge", Name: "edge", Host: "203.0.113.1", Port: 22,
				Proxy: &domain.ProxyConfig{Kind: domain.ProxySocks5, Addr: "127.0.0.1:1080", Username: "u"}},
			"loop-a": {ID: "loop-a", Name: "a", Host: "h", Port: 22,
				Proxy: &domain.ProxyConfig{Kind: domain.ProxyJump, HostID: "loop-b"}},
			"loop-b": {ID: "loop-b", Name: "b", Host: "h", Port: 22,
				Proxy: &domain.ProxyConfig{Kind: domain.ProxyJump, HostID: "loop-a"}},
		},
		creds: map[string]domain.Credentials{
			"dmz":  {Password: "dmz-pass"},
			"edge": {Password: "edge-pass"},
		},
	}
}

func TestRouteForDirect(t *testing.T) {
	s := NewProxyService(newFakeProxyHosts(), nil)
	route, err := s.RouteFor(domain.Host{ID: "plain", Proxy: nil})
	if err != nil || route != nil {
		t.Fatalf("direct host: route=%+v err=%v", route, err)
	}
	route, err = s.RouteFor(domain.Host{ID: "plain", Proxy: &domain.ProxyConfig{Kind: domain.ProxyNone}})
	if err != nil || route != nil {
		t.Fatalf("none kind: route=%+v err=%v", route, err)
	}
}

func TestRouteForJumpChain(t *testing.T) {
	s := NewProxyService(newFakeProxyHosts(), nil)
	route, err := s.RouteFor(domain.Host{ID: "target", Name: "inner",
		Proxy: &domain.ProxyConfig{Kind: domain.ProxyJump, HostID: "dmz"}})
	if err != nil {
		t.Fatalf("route: %v", err)
	}
	// Chain target→dmz→edge: jumps ordered outermost-first, proxy innermost.
	if len(route.Jumps) != 2 {
		t.Fatalf("want 2 jumps, got %+v", route.Jumps)
	}
	if route.Jumps[0].HostID != "edge" || route.Jumps[1].HostID != "dmz" {
		t.Fatalf("jump order wrong: %+v", route.Jumps)
	}
	if route.Proxy == nil || route.Proxy.Kind != "socks5" || route.Proxy.Addr != "127.0.0.1:1080" {
		t.Fatalf("proxy not propagated from edge: %+v", route.Proxy)
	}
	// Each hop carries the jump box's own resolved credentials.
	if route.Jumps[1].Creds.Password != "dmz-pass" {
		t.Fatalf("dmz creds not resolved: %+v", route.Jumps[1].Creds)
	}
	if route.Jumps[1].Addr != "203.0.113.2:2222" {
		t.Fatalf("dmz addr wrong: %s", route.Jumps[1].Addr)
	}
}

func TestRouteForLoopDetection(t *testing.T) {
	s := NewProxyService(newFakeProxyHosts(), nil)
	_, err := s.RouteFor(domain.Host{ID: "loop-a", Name: "a",
		Proxy: &domain.ProxyConfig{Kind: domain.ProxyJump, HostID: "loop-b"}})
	if err == nil {
		t.Fatal("cyclic jump chain accepted")
	}
}

func TestRouteForMissingJumpHost(t *testing.T) {
	s := NewProxyService(newFakeProxyHosts(), nil)
	_, err := s.RouteFor(domain.Host{ID: "x", Name: "x",
		Proxy: &domain.ProxyConfig{Kind: domain.ProxyJump, HostID: "ghost"}})
	if err == nil {
		t.Fatal("missing jump host accepted")
	}
}
