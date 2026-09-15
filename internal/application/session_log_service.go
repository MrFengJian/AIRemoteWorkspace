package application

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// SessionLogService records terminal session output to disk (Phase 8 会话日志):
// the terminal output pump tees every PTY chunk here for sessions the user
// enabled logging on. One log file per session —
//
//	<dataDir>/logs/<host>/<yyyyMMdd-HHmmss>-<session>.log
//
// so concurrent sessions (split panes, repeated tabs) never interleave into
// one file. Raw PTY bytes are written verbatim between two readable marker
// lines; failures to log must never break the terminal, so Start/Write/Stop
// swallow and report file-system errors without panicking.
type SessionLogService struct {
	dataDirs *DataDirService
	// config (may be nil) reads the settings toggles: (disabled, timestamps).
	config func() (disabled bool, timestamps bool)

	mu     sync.Mutex
	files  map[string]*sessionLogFile // sessionID → open log file
}

type sessionLogFile struct {
	f    *os.File
	path string
	// pending holds a trailing partial line when the timestamps setting is
	// on (flushed with a prefix once its newline arrives, or raw on stop).
	pending []byte
}

// SessionLogInfo reports whether a session is being logged and where.
type SessionLogInfo struct {
	Enabled bool   `json:"enabled"`
	Path    string `json:"path"`
}

// NewSessionLogService wires the service to the data directory.
func NewSessionLogService(dataDirs *DataDirService) *SessionLogService {
	return &SessionLogService{dataDirs: dataDirs, files: make(map[string]*sessionLogFile)}
}

// SetConfigProvider wires the settings toggles (called once at startup):
// disabled = the feature is switched off in settings (Start refuses),
// timestamps = every log line is prefixed with HH:MM:SS.
func (s *SessionLogService) SetConfigProvider(fn func() (disabled, timestamps bool)) {
	s.config = fn
}

func (s *SessionLogService) settings() (disabled, timestamps bool) {
	if s.config == nil {
		return false, false
	}
	return s.config()
}

// Disabled reports whether session logging is switched off in settings —
// the UI uses it to explain why Start was refused.
func (s *SessionLogService) Disabled() bool {
	disabled, _ := s.settings()
	return disabled
}

// Dir returns the base directory holding all session logs (created lazily).
func (s *SessionLogService) Dir() string {
	return filepath.Join(s.dataDirs.Current(), "logs")
}

// Start begins logging the session: opens the log file (per-host folder,
// timestamped name) and writes a start marker. Returns the file path.
// Starting twice on one session just re-points at the same open file.
// Refused with an error when the feature is switched off in settings.
func (s *SessionLogService) Start(sessionID, hostName string) (SessionLogInfo, error) {
	if disabled, _ := s.settings(); disabled {
		return SessionLogInfo{}, fmt.Errorf("会话日志已在设置中停用")
	}
	dir := filepath.Join(s.Dir(), sanitizeLogSegment(hostName))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return SessionLogInfo{}, fmt.Errorf("create log dir: %w", err)
	}
	name := fmt.Sprintf("%s-%s.log", time.Now().Format("20060102-150405"), sanitizeLogSegment(sessionID))
	path := filepath.Join(dir, name)

	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.files[sessionID]; ok {
		return SessionLogInfo{Enabled: true, Path: existing.path}, nil
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o644)
	if err != nil {
		return SessionLogInfo{}, fmt.Errorf("create log file: %w", err)
	}
	s.files[sessionID] = &sessionLogFile{f: f, path: path}
	fmt.Fprintf(f, "=== session log started %s ===\n", time.Now().Format("2006-01-02 15:04:05"))
	return SessionLogInfo{Enabled: true, Path: path}, nil
}

// Write appends PTY output to the session's log file; a no-op for sessions
// without logging. Write errors are swallowed (best-effort log). With the
// timestamps setting on, every COMPLETE line is prefixed with HH:MM:SS; a
// trailing fragment is held back until its newline arrives (flushed raw on
// stop).
func (s *SessionLogService) Write(sessionID string, data []byte) {
	if len(data) == 0 {
		return
	}
	s.mu.Lock()
	lf := s.files[sessionID]
	_, timestamps := s.settings()
	s.mu.Unlock()
	if lf == nil {
		return
	}

	out := data
	if timestamps {
		buf := append(lf.pending, data...)
		lf.pending = buf[:0]
		var now = time.Now().Format("15:04:05")
		for {
			i := bytes.IndexByte(buf, '\n')
			if i < 0 {
				lf.pending = append(lf.pending, buf...)
				break
			}
			out = append(out, now...)
			out = append(out, ' ')
			out = append(out, buf[:i+1]...)
			buf = buf[i+1:]
		}
	}
	if len(out) > 0 {
		_, _ = lf.f.Write(out)
	}
}

// Stop ends logging: writes a closing marker and closes the file. Safe to
// call for sessions that never enabled logging, and safe to call twice.
func (s *SessionLogService) Stop(sessionID string) SessionLogInfo {
	s.mu.Lock()
	lf, ok := s.files[sessionID]
	if ok {
		delete(s.files, sessionID)
	}
	s.mu.Unlock()
	if !ok {
		return SessionLogInfo{}
	}
	if len(lf.pending) > 0 {
		_, _ = lf.f.Write(lf.pending)
		lf.pending = nil
	}
	fmt.Fprintf(lf.f, "=== session log stopped %s ===\n", time.Now().Format("2006-01-02 15:04:05"))
	_ = lf.f.Close()
	return SessionLogInfo{Enabled: false, Path: lf.path}
}

// Status reports whether the session is currently being logged (and the
// active file path when it is).
func (s *SessionLogService) Status(sessionID string) SessionLogInfo {
	s.mu.Lock()
	defer s.mu.Unlock()
	if lf, ok := s.files[sessionID]; ok {
		return SessionLogInfo{Enabled: true, Path: lf.path}
	}
	return SessionLogInfo{}
}

// sanitizeLogSegment makes a host name / session id safe as one path segment
// on every platform: separators and Windows-forbidden characters become '-',
// trailing dots/spaces are dropped (Windows), empty input falls back to a
// placeholder. Unicode letters (e.g. Chinese host names) are kept.
func sanitizeLogSegment(name string) string {
	repl := strings.Map(func(r rune) rune {
		switch r {
		case '/', '\\', ':', '*', '?', '"', '<', '>', '|':
			return '-'
		}
		if r < 0x20 || r == 0x7F {
			return '-'
		}
		return r
	}, strings.TrimSpace(name))
	repl = strings.Trim(repl, ". ")
	if repl == "" {
		return "session"
	}
	if len(repl) > 80 {
		repl = repl[:80]
	}
	return repl
}
