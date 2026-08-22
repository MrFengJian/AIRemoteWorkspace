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
//   - WRITE:    file writes (echo >, tee, sed -i), service restart/stop,
//                apt/yum install, docker run/exec, systemctl start/stop.
//   - READ:     everything else (ls, ps, df, cat, grep, uptime, free…)
//
// The patterns are deliberately conservative — when in doubt we escalate
// rather than auto-approve, because these run on production hosts. Two
// precision rules keep common diagnostics READ:
//   - DANGEROUS command names match the *command position* only (the first
//     token of each `;`/`&&`/`||`/`|`-separated segment, after wrappers like
//     sudo/xargs), so `grep rm file` or `man rm` stay READ.
//   - Stream-silencing redirects (`> /dev/null`, `2>&1`) are stripped before
//     WRITE matching, so `cat x 2> /dev/null` stays READ.
func classifyCommand(cmd string) domain.Permission {
	c := strings.ToLower(strings.TrimSpace(cmd))
	if c == "" {
		return domain.PermissionRead
	}

	// 1. DANGEROUS: per-segment command-name / pattern matching.
	for _, seg := range splitSegments(c) {
		if isDangerousSegment(seg) {
			return domain.PermissionDangerous
		}
	}
	// 2. DANGEROUS: whole-string patterns (disk overwrite etc.).
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
// `a && b; c | d` → [a, b, c, d]. Splitting on "|" also covers "||".
func splitSegments(c string) []string {
	return strings.FieldsFunc(c, func(r rune) bool {
		return r == ';' || r == '|' || r == '\n'
	})
}

// isDangerousSegment reports whether one command segment is dangerous: its
// command name (first non-wrapper token) is a dangerous command, or the
// segment matches a multi-word dangerous pattern (chmod 777, kill -9…).
func isDangerousSegment(seg string) bool {
	if name := commandNameOf(seg); name != "" && slices.Contains(dangerousCommandNames, name) {
		return true
	}
	for _, pat := range dangerousSegmentPatterns {
		if pat.MatchString(seg) {
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
// leading path stripped (`/usr/bin/rm` → `rm`).
func commandNameOf(seg string) string {
	for _, f := range strings.Fields(seg) {
		// Skip leading VAR=value assignments (env FOO=1 cmd …).
		if strings.Contains(f, "=") && !strings.HasPrefix(f, "/") && !strings.HasPrefix(f, ".") && !strings.HasPrefix(f, "~") {
			continue
		}
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

// dangerousCommandNames are matched against the effective command name of each
// segment — position matters so `grep rm file` / `echo shutdown` stay READ.
var dangerousCommandNames = []string{
	// File deletion
	"rm", "rmdir",
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
}

// dangerousGlobalPatterns must keep matching the whole command (positionless).
var dangerousGlobalPatterns = []*regexp.Regexp{
	// Overwrite whole disks or raw devices
	regexp.MustCompile(`>\s*/dev/sd`),
	regexp.MustCompile(`>\s*/dev/nvme`),
	regexp.MustCompile(`>\s*/dev/vd`),
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
	// Service control
	regexp.MustCompile(`\bsystemctl\s+(start|stop|restart|reload|enable|disable)\b`),
	regexp.MustCompile(`\bservice\s+\S+\s+(start|stop|restart)\b`),
	// Package management
	regexp.MustCompile(`\b(apt|apt-get)\s+(install|remove|purge|upgrade)\b`),
	regexp.MustCompile(`\byum\s+(install|remove)\b`),
	regexp.MustCompile(`\bdnf\s+(install|remove)\b`),
	// Docker mutations
	regexp.MustCompile(`\bdocker\s+(run|exec|create|build|push|commit)\b`),
	// File creation / move
	regexp.MustCompile(`\bmkdir\b`),
	regexp.MustCompile(`\bcp\b`),
	regexp.MustCompile(`\bmv\b`),
	regexp.MustCompile(`\btouch\b`),
	// Cron / config edits
	regexp.MustCompile(`\bcrontab\b`),
}
