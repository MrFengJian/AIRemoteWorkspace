package application

import (
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"golang.org/x/text/encoding/simplifiedchinese"

	"github.com/ai-remote/workspace/internal/domain"
)

// Host import sources (ParseHostImport / ImportHosts). One parser per legacy
// SSH client; only connection settings travel — secrets never do (Xshell /
// MobaXterm store them encrypted anyway, and the app's security model keeps
// credentials out of SQLite, AGENT.md §9).
const (
	ImportSourceXshell    = "xshell"    // .xsh session files (file or folder)
	ImportSourceMobaXterm = "mobaxterm" // .mxtsessions bookmark exports
	ImportSourcePuTTY     = "putty"     // Windows registry sessions (Windows only)
	ImportSourceSSHConfig = "sshconfig" // OpenSSH ~/.ssh/config
)

// importedTextMaxBytes caps any single session file we are willing to parse —
// session files are tiny; anything larger is not a session export.
const importedTextMaxBytes = 8 << 20

// ImportedHost is the normalized view of one host parsed from an external
// tool. AuthType is password unless the source explicitly referenced a key
// file; the key path is imported best-effort (Xshell stores keys in a
// proprietary vault, so only MobaXterm / PuTTY / ssh_config paths survive).
type ImportedHost struct {
	Name     string
	Host     string
	Port     int
	Username string
	AuthType domain.AuthType
	KeyPath  string
	Group    string // target host group (source folder / bookmark folder)
}

// valid reports whether the row satisfies the app's host requirements
// (mirrors validateHostInput). Invalid rows are shown disabled in the
// preview and refused at import time.
func (h ImportedHost) valid() bool {
	return h.Name != "" && h.Host != "" && h.Username != "" &&
		h.Port >= 1 && h.Port <= 65535 &&
		(h.AuthType == domain.AuthPassword || h.AuthType == domain.AuthKey || h.AuthType == domain.AuthAgent)
}

// HostImportService turns parsed sessions from other SSH clients into real
// hosts, deduplicating against the stored roster (host + port + username).
type HostImportService struct {
	hosts *HostService
}

// NewHostImportService wires the import service over host CRUD.
func NewHostImportService(hosts *HostService) *HostImportService {
	return &HostImportService{hosts: hosts}
}

// ParseImportSource parses sessions from the given source. path is the
// session file / folder (xshell, mobaxterm, sshconfig); an empty path for
// sshconfig means the user's ~/.ssh/config. For putty the path is unused —
// sessions are read from the registry.
func (s *HostImportService) ParseImportSource(source, path string) ([]ImportedHost, error) {
	parsed, err := parseImportSource(source, path)
	if err != nil {
		return nil, err
	}
	// Collapse in-file duplicates (same host:port:user), keeping first.
	seen := map[string]bool{}
	out := parsed[:0]
	for _, h := range parsed {
		key := strings.ToLower(h.Host) + "|" + strconv.Itoa(h.Port) + "|" + h.Username
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, h)
	}
	return out, nil
}

// ImportSummary reports the outcome of an import batch.
type ImportSummary struct {
	Imported int
	Skipped  int // already in the roster (same host + port + username)
	Failed   int // rows that don't satisfy the host requirements
}

// Import parses the source again (stateless — no server-side session state)
// and creates the hosts selected by index. Rows are deduplicated against the
// stored roster and within the batch; selected-but-invalid rows are counted
// as failed.
func (s *HostImportService) Import(source, path string, indexes []int) (ImportSummary, error) {
	parsed, err := parseImportSource(source, path)
	if err != nil {
		return ImportSummary{}, err
	}
	pick := map[int]bool{}
	for _, i := range indexes {
		pick[i] = true
	}

	existing, err := s.hosts.List()
	if err != nil {
		return ImportSummary{}, err
	}
	known := map[string]bool{}
	for _, h := range existing {
		known[hostDedupeKey(h.Host, h.Port, h.Username)] = true
	}

	var summary ImportSummary
	for i, h := range parsed {
		if !pick[i] {
			continue
		}
		if h.Name == "" {
			h.Name = h.Host // last-resort display name
		}
		if !h.valid() {
			summary.Failed++
			continue
		}
		key := hostDedupeKey(h.Host, h.Port, h.Username)
		if known[key] {
			summary.Skipped++
			continue
		}
		if _, err := s.hosts.Create(CreateHostInput{
			Name:     h.Name,
			Host:     h.Host,
			Port:     h.Port,
			Username: h.Username,
			AuthType: h.AuthType,
			KeyPath:  h.KeyPath,
			Group:    h.Group,
		}); err != nil {
			summary.Failed++
			continue
		}
		known[key] = true
		summary.Imported++
	}
	return summary, nil
}

// hostDedupeKey is the roster-uniqueness key: two entries connecting to the
// same account on the same machine are the same host regardless of label.
func hostDedupeKey(host string, port int, username string) string {
	return strings.ToLower(host) + "|" + strconv.Itoa(port) + "|" + username
}

func parseImportSource(source, path string) ([]ImportedHost, error) {
	switch source {
	case ImportSourceXshell:
		return parseXshell(path)
	case ImportSourceMobaXterm:
		return parseMobaXterm(path)
	case ImportSourcePuTTY:
		return parsePuTTY()
	case ImportSourceSSHConfig:
		return parseSSHConfig(path)
	default:
		return nil, fmt.Errorf("unknown import source %q", source)
	}
}

// --- shared text helpers -------------------------------------------------

// readTextFile reads a session file tolerating the encodings real tools
// write: UTF-8 (± BOM), UTF-16 LE/BE (Xshell's default on modern versions)
// and GBK/ANSI as the non-UTF-8 fallback (Chinese Windows installs).
func readTextFile(path string) (string, error) {
	st, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if st.Size() > importedTextMaxBytes {
		return "", fmt.Errorf("%s: file too large to be a session export", filepath.Base(path))
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return decodeSessionText(raw), nil
}

// decodeSessionText detects BOM / encoding and returns the decoded text.
func decodeSessionText(raw []byte) string {
	switch {
	case len(raw) >= 2 && raw[0] == 0xFF && raw[1] == 0xFE: // UTF-16 LE BOM
		return string(utf16.Decode(leU16(raw[2:])))
	case len(raw) >= 2 && raw[0] == 0xFE && raw[1] == 0xFF: // UTF-16 BE BOM
		return string(utf16.Decode(beU16(raw[2:])))
	case len(raw) >= 3 && raw[0] == 0xEF && raw[1] == 0xBB && raw[2] == 0xBF:
		return string(raw[3:])
	}
	if utf8.Valid(raw) {
		return string(raw)
	}
	// Legacy ANSI content: GBK covers the common case; undecodable bytes
	// become replacement runes, which at worst garble a display name.
	dec := simplifiedchinese.GBK.NewDecoder()
	out, err := dec.Bytes(raw)
	if err != nil {
		return string(raw)
	}
	return string(out)
}

func leU16(b []byte) []uint16 {
	u := make([]uint16, len(b)/2)
	for i := range u {
		u[i] = uint16(b[2*i]) | uint16(b[2*i+1])<<8
	}
	return u
}

func beU16(b []byte) []uint16 {
	u := make([]uint16, len(b)/2)
	for i := range u {
		u[i] = uint16(b[2*i])<<8 | uint16(b[2*i+1])
	}
	return u
}

// iniSection is one parsed INI block with its keys in file order (order
// matters for MobaXterm bookmark lists).
type iniSection struct {
	name string
	keys [][2]string // key, value
}

// parseINI is a minimal INI reader: [section], key=value, ; / # comments,
// leading/trailing whitespace trimmed. Keys before any section land in "".
func parseINI(text string) []iniSection {
	var secs []iniSection
	cur := iniSection{name: ""}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if line == "" || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			secs = append(secs, cur)
			cur = iniSection{name: line[1 : len(line)-1]}
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		cur.keys = append(cur.keys, [2]string{strings.TrimSpace(key), strings.TrimSpace(value)})
	}
	secs = append(secs, cur)
	return secs
}

// getINI returns the last value of key in the named section ("" when absent).
func getINI(secs []iniSection, section, key string) string {
	var v string
	for _, s := range secs {
		if !strings.EqualFold(s.name, section) {
			continue
		}
		for _, kv := range s.keys {
			if strings.EqualFold(kv[0], key) {
				v = kv[1]
			}
		}
	}
	return v
}

// --- Xshell (.xsh) --------------------------------------------------------

// parseXshell reads one .xsh session file or a folder of them (Xshell's
// Sessions directory — subfolders become host groups). Only SSH sessions are
// imported; telnet / serial / sftp profiles are silently skipped.
func parseXshell(path string) ([]ImportedHost, error) {
	st, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	var files []string
	var root string
	if st.IsDir() {
		root = path
		err = filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !d.IsDir() && strings.EqualFold(filepath.Ext(p), ".xsh") {
				files = append(files, p)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	} else {
		if !strings.EqualFold(filepath.Ext(path), ".xsh") {
			return nil, fmt.Errorf("%s: not an .xsh session file", filepath.Base(path))
		}
		files = []string{path}
		root = filepath.Dir(path)
	}
	if len(files) == 0 {
		return nil, errors.New("no .xsh session files found")
	}
	sort.Strings(files)

	var out []ImportedHost
	for _, f := range files {
		h, ok := parseXshellFile(f)
		if !ok {
			continue // non-SSH protocol or unreadable — skip quietly
		}
		// Subfolder under the picked root becomes the host group.
		if rel, err := filepath.Rel(root, filepath.Dir(f)); err == nil && rel != "." {
			h.Group = filepath.ToSlash(rel)
		}
		out = append(out, h)
	}
	if len(out) == 0 {
		return nil, errors.New("no importable SSH sessions found in the .xsh files")
	}
	return out, nil
}

// parseXshellFile extracts one session. ok is false for non-SSH protocols.
func parseXshellFile(path string) (ImportedHost, bool) {
	text, err := readTextFile(path)
	if err != nil {
		return ImportedHost{}, false
	}
	secs := parseINI(text)
	protocol := strings.ToUpper(getINI(secs, "CONNECTION", "Protocol"))
	if protocol != "" && protocol != "SSH" {
		return ImportedHost{}, false
	}
	host := getINI(secs, "CONNECTION", "Host")
	port, _ := strconv.Atoi(getINI(secs, "CONNECTION", "Port"))
	user := getINI(secs, "CONNECTION:AUTHENTICATION", "UserName")
	userKey := getINI(secs, "CONNECTION:AUTHENTICATION", "UserKey")

	h := ImportedHost{
		Name:     strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)),
		Host:     host,
		Port:     port,
		Username: user,
		AuthType: domain.AuthPassword,
	}
	if h.Port == 0 {
		h.Port = 22
	}
	// A referenced user key means key auth; Xshell keeps the key material in
	// its own vault (.xkp), so no portable path exists — the user picks the
	// key file in the host settings after import.
	if userKey != "" {
		h.AuthType = domain.AuthKey
	}
	return h, h.Host != ""
}

// --- MobaXterm (.mxtsessions) ----------------------------------------------

// parseMobaXterm reads a .mxtsessions bookmark export (the [Bookmarks] /
// [Bookmarks_N] sections also match MobaXterm.ini, so picking that file
// works too). Session lines look like:
//
//	Name=#109#0%host%port%user%…%-fields…%#MobaFont%…#0##-1
//
// The first %-group's leading number is the session type (0 = SSH); field
// indices inside the group: 1 host, 2 port, 3 user, 14 identity file.
func parseMobaXterm(path string) ([]ImportedHost, error) {
	if st, err := os.Stat(path); err != nil || st.IsDir() {
		if err == nil {
			return nil, fmt.Errorf("%s: please pick a .mxtsessions file, not a folder", filepath.Base(path))
		}
		return nil, err
	}
	text, err := readTextFile(path)
	if err != nil {
		return nil, err
	}

	var out []ImportedHost
	for _, sec := range parseINI(text) {
		if !strings.EqualFold(sec.name, "Bookmarks") &&
			!strings.HasPrefix(strings.ToLower(sec.name), "bookmarks_") {
			continue
		}
		group := ""
		for _, kv := range sec.keys {
			if strings.EqualFold(kv[0], "SubRep") {
				group = strings.ReplaceAll(filepath.ToSlash(kv[1]), "\\", "/")
				break
			}
		}
		for _, kv := range sec.keys {
			if kv[0] == "SubRep" || kv[0] == "ImgNum" || kv[0] == "Private" {
				continue
			}
			h, ok := parseMobaSession(kv[0], kv[1])
			if !ok {
				continue
			}
			h.Group = group
			out = append(out, h)
		}
	}
	if len(out) == 0 {
		return nil, errors.New("no SSH sessions found in the .mxtsessions file")
	}
	return out, nil
}

// parseMobaSession decodes one bookmark line. ok is false for non-SSH types
// and unrecognizable values.
func parseMobaSession(name, value string) (ImportedHost, bool) {
	// Find the segment whose first %-field is a session type we know.
	var fields []string
	for _, seg := range strings.Split(value, "#") {
		f := strings.Split(seg, "%")
		if len(f) > 1 && f[0] == "0" { // 0 = SSH (4 RDP, 5 VNC, 7 SFTP, 11 browser…)
			fields = f
			break
		}
	}
	if len(fields) < 2 || fields[1] == "" {
		return ImportedHost{}, false
	}

	port := 22
	if len(fields) > 2 {
		if p, err := strconv.Atoi(fields[2]); err == nil && p > 0 {
			port = p
		}
	}
	h := ImportedHost{
		Name:     name,
		Host:     fields[1],
		Port:     port,
		Username: fields[3],
		AuthType: domain.AuthPassword,
	}
	if len(fields) > 3 {
		h.Username = mobaUnescape(fields[3])
	}
	if len(fields) > 14 {
		h.KeyPath = mobaUnescape(fields[14])
	}
	if h.KeyPath == "" {
		// Field indices drift between MobaXterm versions; fall back to
		// "whichever later field looks like a filesystem path".
		for _, f := range fields[4:] {
			if looksLikePath(f) {
				h.KeyPath = mobaUnescape(f)
				break
			}
		}
	}
	if h.KeyPath != "" {
		h.AuthType = domain.AuthKey
	}
	return h, h.Host != ""
}

// mobaUnescape reverses MobaXterm's reserved-character escapes.
func mobaUnescape(s string) string {
	r := strings.NewReplacer(
		"_CurrentDrive_", "C:",
		"__DIEZE__", "#",
		"__PTVIRG__", ";",
		"__PIPE__", "|",
		"__DBLQUO__", `"`,
	)
	return r.Replace(s)
}

// looksLikePath reports whether a bookmark field is plausibly a file path
// (used only for the best-effort identity-file fallback).
func looksLikePath(s string) bool {
	if s == "" || len(s) > 500 {
		return false
	}
	if strings.HasPrefix(s, "_CurrentDrive_") {
		return true
	}
	if i := strings.IndexAny(s, `/\`); i > 0 {
		return true
	}
	if len(s) > 2 && s[1] == ':' {
		return true
	}
	return false
}

// --- OpenSSH config ---------------------------------------------------------

// parseSSHConfig imports literal Host entries from an OpenSSH config. Empty
// path means the user's ~/.ssh/config.
func parseSSHConfig(path string) ([]ImportedHost, error) {
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		path = filepath.Join(home, ".ssh", "config")
	}
	if _, err := os.Stat(path); err != nil {
		return nil, errors.New("SSH config not found (pick a file or create ~/.ssh/config)")
	}
	text, err := readTextFile(path)
	if err != nil {
		return nil, err
	}
	return parseSSHConfigText(text)
}

// parseSSHConfigText does the actual parsing (split from file I/O for tests).
// Wildcard / negated patterns are skipped (they're match rules, not
// machines), but settings from catch-all blocks (`Host *`) are applied as
// defaults to every imported entry — the common "Host * → User me /
// IdentityFile" idiom keeps working. Match blocks are ignored.
func parseSSHConfigText(text string) ([]ImportedHost, error) {
	type block struct {
		patterns []string
		settings map[string]string
	}
	var blocks []block
	var cur *block
	for _, line := range strings.Split(text, "\n") {
		if i := strings.Index(line, "#"); i >= 0 {
			line = line[:i]
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		switch strings.ToLower(fields[0]) {
		case "host":
			blocks = append(blocks, block{patterns: fields[1:], settings: map[string]string{}})
			cur = &blocks[len(blocks)-1]
		case "match":
			cur = nil // conditional block — out of scope for import
		default:
			if cur != nil && len(fields) >= 2 {
				key := strings.ToLower(fields[0])
				if _, seen := cur.settings[key]; !seen { // first occurrence wins (ssh semantics)
					cur.settings[key] = strings.Join(fields[1:], " ")
				}
			}
		}
	}

	// Catch-all defaults from `Host *` (and only that) blocks.
	defaults := map[string]string{}
	for _, b := range blocks {
		if len(b.patterns) == 1 && b.patterns[0] == "*" {
			for k, v := range b.settings {
				if _, ok := defaults[k]; !ok {
					defaults[k] = v
				}
			}
		}
	}

	// Import literal blocks; first block for a given pattern wins (keeps the
	// output deterministic and mirrors ssh's first-match-wins ordering).
	entries := map[string]ImportedHost{}
	var order []string
	for _, b := range blocks {
		if len(b.patterns) == 0 || !allLiteral(b.patterns) {
			continue
		}
		for _, pattern := range b.patterns {
			h := ImportedHost{
				Name:     pattern,
				Port:     22,
				AuthType: domain.AuthPassword,
			}
			applySetting := func(key string) {
				v := b.settings[key]
				if v == "" {
					v = defaults[key]
				}
				switch key {
				case "hostname":
					h.Host = v
				case "user":
					h.Username = v
				case "port":
					if p, err := strconv.Atoi(v); err == nil {
						h.Port = p
					}
				case "identityfile":
					if v != "" {
						h.KeyPath = expandHome(v)
						h.AuthType = domain.AuthKey
					}
				}
			}
			applySetting("hostname")
			applySetting("user")
			applySetting("port")
			applySetting("identityfile")
			if h.Host == "" {
				h.Host = pattern
			}
			if _, dup := entries[pattern]; !dup {
				entries[pattern] = h
				order = append(order, pattern)
			}
		}
	}
	if len(order) == 0 {
		return nil, errors.New("no literal Host entries found in the SSH config")
	}
	out := make([]ImportedHost, 0, len(order))
	for _, name := range order {
		out = append(out, entries[name])
	}
	return out, nil
}

// allLiteral reports whether every pattern is a plain host name (no * ? !).
func allLiteral(patterns []string) bool {
	for _, p := range patterns {
		if strings.ContainsAny(p, "*?!") {
			return false
		}
	}
	return true
}

func expandHome(p string) string {
	if p == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			return home
		}
	}
	if strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, p[2:])
		}
	}
	return p
}

// puttySessionName decodes a registry subkey name (URL-escaped by PuTTY).
func puttySessionName(subkey string) string {
	if n, err := url.PathUnescape(subkey); err == nil && n != "" {
		return n
	}
	return subkey
}
