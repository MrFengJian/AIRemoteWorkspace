package application

import (
	"fmt"
	"net"
	"strconv"

	"github.com/ai-remote/workspace/internal/domain"
)

// ProxyService resolves a host's pre-SSH reachability (domain.ProxyConfig)
// into a concrete domain.SshRoute for the connection layer: direct, a jump
// chain (堡垒机, each hop itself possibly behind another jump or proxy), or
// an HTTP CONNECT / SOCKS5 proxy.
//
// It implements SshRouteResolver and is injected into the SSH connection
// layer (manager, tunnels, SFTP) — every dial path then routes identically.
type ProxyService struct {
	hosts   hostSource
	secrets *SecretService
}

// hostSource is the slice of HostService the resolver needs (satisfied by
// *HostService; an interface so tests can fake it).
type hostSource interface {
	Get(id string) (domain.Host, error)
	ResolveCredentials(host domain.Host, creds domain.Credentials) (domain.Credentials, error)
}

// Compile-time check that *HostService provides the host source surface.
var _ hostSource = (*HostService)(nil)

// NewProxyService wires the resolver to host lookup + the OS vault.
func NewProxyService(hosts hostSource, secrets *SecretService) *ProxyService {
	return &ProxyService{hosts: hosts, secrets: secrets}
}

// RouteFor resolves host's proxy config into a route plan. Returns
// (nil, nil) for a direct connection — callers treat that as "dial directly".
func (s *ProxyService) RouteFor(host domain.Host) (*domain.SshRoute, error) {
	if !host.Proxy.HasTransport() {
		return nil, nil
	}
	return s.buildRoute(host, map[string]bool{host.ID: true})
}

// buildRoute recursively resolves host's route. visited holds every host id
// already on the path (target + jumps) so a cyclic jump configuration fails
// fast instead of dialing forever.
func (s *ProxyService) buildRoute(host domain.Host, visited map[string]bool) (*domain.SshRoute, error) {
	p := host.Proxy
	switch p.Kind {
	case domain.ProxyJump:
		if p.HostID == "" {
			return nil, fmt.Errorf("host %q: 跳板机未选择", host.Name)
		}
		jump, err := s.hosts.Get(p.HostID)
		if err != nil {
			return nil, fmt.Errorf("host %q: 跳板机不可用: %w", host.Name, err)
		}
		if visited[jump.ID] {
			return nil, fmt.Errorf("host %q: 跳板链存在环路（%s 已在链上）", host.Name, jump.Name)
		}
		visited[jump.ID] = true

		creds, err := s.hosts.ResolveCredentials(jump, domain.Credentials{})
		if err != nil {
			return nil, fmt.Errorf("host %q: 解析跳板机凭据失败: %w", host.Name, err)
		}

		route := &domain.SshRoute{}
		// The jump box itself may sit behind another jump / proxy — recurse;
		// its route becomes the OUTER part of ours (how to reach the box).
		if jump.Proxy.HasTransport() {
			outer, err := s.buildRoute(jump, visited)
			if err != nil {
				return nil, err
			}
			route.Jumps = outer.Jumps
			route.Proxy = outer.Proxy
		}
		route.Jumps = append(route.Jumps, domain.SshJump{
			HostID:   jump.ID,
			Addr:     net.JoinHostPort(jump.Host, strconv.Itoa(jump.Port)),
			Username: jump.Username,
			Creds:    creds,
		})
		return route, nil

	case domain.ProxyHTTP, domain.ProxySocks5:
		if p.Addr == "" {
			return nil, fmt.Errorf("host %q: 代理地址为空", host.Name)
		}
		ep := &domain.ProxyEndpoint{
			Kind:     string(p.Kind),
			Addr:     p.Addr,
			Username: p.Username,
		}
		if s.secrets != nil {
			if pass, err := s.secrets.GetHostSecret(host.ID, SecretProxyPassword); err == nil {
				ep.Password = string(pass)
			}
		}
		return &domain.SshRoute{Proxy: ep}, nil

	default:
		return nil, fmt.Errorf("host %q: 未知的代理类型 %q", host.Name, p.Kind)
	}
}
