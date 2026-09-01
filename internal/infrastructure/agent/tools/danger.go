package tools

import (
	"regexp"
	"slices"
	"strings"

	"github.com/ai-remote/workspace/internal/domain"
)

// classifyCommand inspects a shell command string and returns the permission
// tier it requires (AGENT.md §14):
//   - DANGEROUS: rm, shutdown, reboot, iptables, mkfs, dd, chmod 777,
//                docker prune/rm, kill -9, userdel, etc.
//   - WRITE:    file writes (echo >, tee, sed -i, truncate), service
//               restart/stop, apt/yum install, docker run/exec,
//               systemctl start/stop.
//   - READ:     everything else (ls, ps, df, cat, grep, uptime, free…)
//
// The patterns are deliberately conservative — when in doubt we escalate
// rather than auto-approve, because these run on production hosts. The rules
// that keep precision without opening evasion holes:
//   - DANGEROUS command names match the *command position* only (the first
//     token of each `;`/`&&`/`||`/`|`-separated segment, after wrappers like
//     sudo/xargs), so `grep rm file` or `man rm` stay READ.
//   - Stream-silencing redirects (`> /dev/null`, `2>&1`) are stripped before
//     WRITE matching, so `cat x 2> /dev/null` stays READ.
//   - Shell interpreters are classified by their PAYLOAD, not their name:
//     `bash -c "rm -rf x"` and `eval "…"` recurse into the quoted code.
//     `source FILE`, `sh FILE` and `anything | sh` run content we cannot
//     inspect, so they escalate to DANGEROUS.
//   - Command substitution (`$(…)`, backticks) and variable indirection
//     (`c=rm; $c -rf /`) hide the executed command from position matching —
//     a dangerous token anywhere inside them escalates the whole command.
func classifyCommand(cmd string) domain.Permission {
	return classifyAt(cmd, 0)
}

// maxClassifyDepth bounds recursion through shell-interpreter payloads
// (bash -c → eval → …); real commands never nest this deep.
const maxClassifyDepth = 3

func classifyAt(cmd string, depth int) domain.Permission {
	c := strings.ToLower(strings.TrimSpace(cmd))
	if c == "" {
		return domain.PermissionRead
	}

	// 1. DANGEROUS: per-segment command-name / pattern matching.
	for i, seg := range splitSegments(c) {
		if isDangerousSegment(seg, depth) {
			return domain.PermissionDangerous
		}
		// Piping into a shell runs unreviewable content (`curl … | bash`,
		// `base64 -d … | sh`) — escalate regardless of the payload.
		if i > 0 && shellInterpreters[commandNameOf(seg)] {
			return domain.PermissionDangerous
		}
	}
	// 2. DANGEROUS: whole-string patterns (disk overwrite, fork bombs).
	for _, pat := range dangerousGlobalPatterns {
		if pat.MatchString(c) {
			return domain.PermissionDangerous
		}
	}
	// 3. WRITE: match on the redirect-stripped command.
	cNoRedir := safeRedirectRe.ReplaceAllString(c, " ")
	for _, pat := range writePatterns {
		if pat.MatchString(cNoRedir) {
			return domain.PermissionWrite
		}
	}
	return domain.PermissionRead
}

// splitSegments breaks a command into its individually-executed parts:
// `a && b; c | d` → [a, b, c, d]. Splitting on "|" also covers "||"; "&&" is
// normalized to ";" first (a quoted "&&" gets split too — an accepted
// over-approximation that errs toward escalating).
func splitSegments(c string) []string {
	return strings.FieldsFunc(strings.ReplaceAll(c, "&&", ";"), func(r rune) bool {
		return r == ';' || r == '|' || r == '\n'
	})
}

// shellInterpreters execute their argument (or stdin) as code rather than
// being interesting commands themselves.
var shellInterpreters = map[string]bool{
	"bash": true, "sh": true, "dash": true, "zsh": true,
	"eval": true, "source": true, ".": true,
}

// isDangerousSegment reports whether one command segment is dangerous: its
// command name (first non-wrapper token) is dangerous or an unknowable
// indirection, it matches a multi-word dangerous pattern (chmod 777,
// kill -9…), or it hides dangerous code inside an interpreter payload or a
// command substitution.
func isDangerousSegment(seg string, depth int) bool {
	if name := commandNameOf(seg); name != "" {
		// Variable indirection (`c=rm; $c -rf /`, `$(cmd) …`): what actually
		// runs is unknowable statically — escalate instead of guessing.
		if strings.HasPrefix(name, "$") {
			return true
		}
		if slices.Contains(dangerousCommandNames, name) {
			return true
		}
		// Interpreters and direct script execution (`./deploy.sh`, `sh x.sh`,
		// `source x`) run content that must itself be classified; when there
		// is no inspectable payload, the execution itself is the risk.
		if shellInterpreters[name] || strings.HasSuffix(name, ".sh") {
			if payload, ok := interpreterPayload(seg); !ok {
				return true
			} else if depth < maxClassifyDepth &&
				classifyAt(payload, depth+1) == domain.PermissionDangerous {
				return true
			}
		}
	}
	for _, pat := range dangerousSegmentPatterns {
		if pat.MatchString(seg) {
			return true
		}
	}
	// Command substitution hides commands mid-segment (`echo $(rm -rf /x)`)
	// where command-position matching can't see them — scan every token.
	if strings.Contains(seg, "$(") || strings.Contains(seg, "`") {
		if hasDangerousToken(seg) {
			return true
		}
	}
	return false
}

// commandWrappers are prefixes that don't change which command runs —
// `sudo rm`, `xargs rm`, `nice rm` are all rm.
var commandWrappers = map[string]bool{
	"sudo": true, "nohup": true, "xargs": true, "time": true,
	"env": true, "stdbuf": true, "nice": true, "watch": true,
}

// commandNameOf returns the effective command name of a segment: the first
// token that is neither an env assignment (VAR=…) nor a wrapper, with any
// leading path stripped (`/usr/bin/rm` → `rm`) and quote artifacts removed
// (`"rm"`, or a trailing quote left by a `&&`-split inside quotes).
func commandNameOf(seg string) string {
	for _, f := range strings.Fields(seg) {
		// Skip leading VAR=value assignments (env FOO=1 cmd …).
		if strings.Contains(f, "=") && !strings.HasPrefix(f, "/") && !strings.HasPrefix(f, ".") && !strings.HasPrefix(f, "~") {
			continue
		}
		f = strings.Trim(f, "\"'`")
		name := f
		if i := strings.LastIndex(name, "/"); i >= 0 {
			name = name[i+1:]
		}
		if commandWrappers[name] {
			continue
		}
		return name
	}
	return ""
}

// interpreterPayload extracts the code a shell interpreter would run for
// segments like `bash -c "…"` or `eval "…"`: everything after the `-c` flag
// (or after `eval`), outer quotes stripped. ok is false when there is no
// static payload to inspect (`source FILE`, `sh FILE`) — the caller treats
// that as dangerous.
func interpreterPayload(seg string) (string, bool) {
	type token struct {
		tok   string
		start int
	}
	var fields []token
	for i := 0; i < len(seg); {
		for i < len(seg) && (seg[i] == ' ' || seg[i] == '\t') {
			i++
		}
		start := i
		for i < len(seg) && seg[i] != ' ' && seg[i] != '\t' {
			i++
		}
		if i > start {
			fields = append(fields, token{seg[start:i], start})
		}
	}
	for i, f := range fields {
		if f.tok == "-c" {
			if i+1 < len(fields) {
				return unquote(seg[fields[i+1].start:]), true
			}
			return "", true // bare `bash -c` — degenerate, nothing to run
		}
		if f.tok == "eval" {
			rest := seg[f.start+len(f.tok):]
			if strings.TrimSpace(rest) == "" {
				return "", true // bare `eval` — nothing to run
			}
			return unquote(rest), true
		}
	}
	return "", false
}

// unquote strips surrounding quote characters from an extracted payload.
func unquote(s string) string {
	return strings.Trim(strings.TrimSpace(s), "\"'`")
}

// hasDangerousToken scans every token of a segment (not just the command
// position) for a dangerous command name — used when substitution markers
// mean dangerous code may hide anywhere in the text.
func hasDangerousToken(seg string) bool {
	for _, f := range strings.Fields(seg) {
		f = strings.Trim(f, "\"'`$(){}`")
		if i := strings.LastIndex(f, "/"); i >= 0 {
			f = f[i+1:]
		}
		if f != "" && slices.Contains(dangerousCommandNames, f) {
			return true
		}
	}
	return false
}

// dangerousCommandNames are matched against the effective command name of each
// segment — position matters so `grep rm file` / `echo shutdown` stay READ.
var dangerousCommandNames = []string{
	// File deletion
	"rm", "rmdir", "shred", "wipefs",
	// System power
	"shutdown", "reboot", "poweroff", "halt", "init",
	// Network / firewall
	"iptables", "nft", "ufw", "firewall-cmd",
	// Filesystem
	"mkfs", "mkfs.ext4", "mkfs.xfs", "dd", "fdisk", "parted", "mount", "umount",
	// Users / permissions
	"userdel", "usermod", "passwd", "chage", "chown",
	// Processes
	"killall", "pkill",
	// Bootloader / firmware
	"grub-install", "sysctl",
}

// dangerousSegmentPatterns are multi-word dangerous forms matched within one
// command segment.
var dangerousSegmentPatterns = []*regexp.Regexp{
	regexp.MustCompile(`\bdocker\s+(system\s+)?prune\b`),
	regexp.MustCompile(`\bdocker\s+(rm|rmi|volume\s+rm|network\s+rm)\b`),
	regexp.MustCompile(`\bkubectl\s+delete\b`),
	regexp.MustCompile(`\bchmod\s+777\b`),
	regexp.MustCompile(`\bkill\s+-9\b`),
	regexp.MustCompile(`\binit\s+0\b`),
	// Mass deletion via find (deletes are irreversible regardless of path).
	regexp.MustCompile(`\bfind\b[^|;]*\s-delete\b`),
}

// dangerousGlobalPatterns must keep matching the whole command (positionless).
var dangerousGlobalPatterns = []*regexp.Regexp{
	// Overwrite whole disks or raw devices
	regexp.MustCompile(`>\s*/dev/sd`),
	regexp.MustCompile(`>\s*/dev/nvme`),
	regexp.MustCompile(`>\s*/dev/vd`),
	// Fork bomb (`:(){ :|:& };:`) — segment splitting shreds it, so it must
	// be matched positionless.
	regexp.MustCompile(`:\s*\(\s*\)\s*\{`),
}

// safeRedirectRe matches stream-silencing/duplicating redirects that touch no
// files: `> /dev/null`, `2> /dev/null`, `2>/dev/null`, `>> /dev/null`,
// `2>&1`, `>&2` (stderr→stderr is a no-op for permissions). Stripped before
// WRITE matching so read-only diagnostics keep their READ tier.
var safeRedirectRe = regexp.MustCompile(`(\d*)>{1,2}\s*(/dev/null\b|&1|&2)`)

// writePatterns match commands that modify state but are recoverable.
var writePatterns = []*regexp.Regexp{
	// File writes (after safe-redirect stripping, a remaining `>` writes a file)
	regexp.MustCompile(`>{1,2}\s*\S`),
	regexp.MustCompile(`\btee\b`),
	regexp.MustCompile(`\bsed\s+-i\b`),
	// Data destruction short of system damage (zeroes a file — same tier as `>`).
	regexp.MustCompile(`\btruncate\b`),
	// Service control
	regexp.MustCompile(`\bsystemctl\s+(start|stop|restart|reload|enable|disable)\b`),
	regexp.MustCompile(`\bservice\s+\S+\s+(start|stop|restart)\b`),
	// Package management
	regexp.MustCompile(`\b(apt|apt-get)\s+(install|remove|purge|upgrade)\b`),
	regexp.MustCompile(`\byum\s+(install|remove)\b`),
	regexp.MustCompile(`\bdnf\s+(install|remove)\b`),
	// Docker mutations
	regexp.MustCompile(`\bdocker\s+(run|exec|create|build|push|commit)\b`),
	// Docker container lifecycle control (stop/start/restart changes runtime state)
	regexp.MustCompile(`\bdocker\s+(stop|start|restart|pause|unpause|kill|update|rename|tag|save|load)\b`),
	// Docker compose / swarm mutations
	regexp.MustCompile(`\bdocker\s+(compose|stack|service|swarm|node)\s+(up|down|start|stop|restart|kill|create|build|rm|remove|scale|deploy|apply|update|leave|promote|demote)\b`),
	// File creation / move
	regexp.MustCompile(`\bmkdir\b`),
	regexp.MustCompile(`\bcp\b`),
	regexp.MustCompile(`\bmv\b`),
	regexp.MustCompile(`\btouch\b`),
	// Cron / config edits
	regexp.MustCompile(`\bcrontab\b`),
}
