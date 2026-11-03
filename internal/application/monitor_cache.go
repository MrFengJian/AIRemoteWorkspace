package application

import "time"

// Shared result cache for the monitor collectors. The monitor panel polls
// through the Wails bindings while the MCP host_stats tool reads the same
// service; both (and every terminal tab of one host) hit one entry per
// host+section instead of each running its own exec round-trip.

// monitorCacheEntry is one cached section payload with its collection time.
// data is typed by the accessor's generic parameter.
type monitorCacheEntry struct {
	at   time.Time
	data any
}

// scopeFor resolves the cache scope of a session: every session of one host
// shares the host scope (this is what makes the panel and MCP share data);
// host-less local sessions fall back to their own id.
func (s *MonitorService) scopeFor(sessionID string) string {
	if s.connect != nil {
		if h, ok := s.connect.HostOfSession(sessionID); ok && h.ID != "" {
			return "host:" + h.ID
		}
	}
	return "session:" + sessionID
}

// monitorCacheGet returns the cached payload when it is younger than the
// TTL. A missing or stale entry means "collect again".
func monitorCacheGet[T any](s *MonitorService, scope, section string) (T, bool) {
	s.mu.Lock()
	e, ok := s.cache[scope+"|"+section]
	s.mu.Unlock()
	var zero T
	if !ok || s.now().Sub(e.at) >= s.cacheTTL {
		return zero, false
	}
	v, ok := e.data.(T)
	return v, ok
}

// monitorCachePut stores a collection result. Errors are never stored, so a
// failed host is retried on every call instead of sticking for the TTL.
func monitorCachePut[T any](s *MonitorService, scope, section string, data T) {
	s.mu.Lock()
	s.cache[scope+"|"+section] = monitorCacheEntry{at: s.now(), data: data}
	s.mu.Unlock()
}
