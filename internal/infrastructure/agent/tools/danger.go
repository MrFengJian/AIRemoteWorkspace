package tools

import (
	"regexp"
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
// rather than auto-approve, because these run on production hosts.
func classifyCommand(cmd string) domain.Permission {
	c := strings.ToLower(strings.TrimSpace(cmd))
	if c == "" {
		return domain.PermissionRead
	}

	for _, pat := range dangerousPatterns {
		if pat.MatchString(c) {
			return domain.PermissionDangerous
		}
	}
	for _, pat := range writePatterns {
		if pat.MatchString(c) {
			return domain.PermissionWrite
		}
	}
	return domain.PermissionRead
}

// dangerousPatterns match commands that can cause irreversible damage.
// Compiled once at init.
var dangerousPatterns = []*regexp.Regexp{
	// File deletion
	regexp.MustCompile(`\brm\b`),
	regexp.MustCompile(`\brmdir\b`),
	// System power
	regexp.MustCompile(`\bshutdown\b`),
	regexp.MustCompile(`\breboot\b`),
	regexp.MustCompile(`\bpoweroff\b`),
	regexp.MustCompile(`\bhalt\b`),
	regexp.MustCompile(`\binit\s+0\b`),
	// Network / firewall
	regexp.MustCompile(`\biptables\b`),
	regexp.MustCompile(`\bnft\b`),
	regexp.MustCompile(`\bufw\b`),
	// Filesystem
	regexp.MustCompile(`\bmkfs\b`),
	regexp.MustCompile(`\bdd\b`),
	regexp.MustCompile(`\bfdisk\b`),
	regexp.MustCompile(`\bparted\b`),
	regexp.MustCompile(`\bmount\b`),
	regexp.MustCompile(`\bumount\b`),
	// Mass operations
	regexp.MustCompile(`\bdocker\s+prune\b`),
	regexp.MustCompile(`\bdocker\s+(rm|rmi|volume\s+rm|network\s+rm)\b`),
	regexp.MustCompile(`\bkubectl\s+delete\b`),
	// Users / permissions
	regexp.MustCompile(`\buserdel\b`),
	regexp.MustCompile(`\busermod\b`),
	regexp.MustCompile(`\bpasswd\b`),
	regexp.MustCompile(`\bchmod\s+777\b`),
	regexp.MustCompile(`\bchown\b`),
	// Processes
	regexp.MustCompile(`\bkillall\b`),
	regexp.MustCompile(`\bkill\s+-9\b`),
	regexp.MustCompile(`\bpkill\b`),
	// Overwrite whole disks or critical dirs
	regexp.MustCompile(`>\s*/dev/sd`),
	regexp.MustCompile(`>\s*/dev/nvme`),
}

// writePatterns match commands that modify state but are recoverable.
var writePatterns = []*regexp.Regexp{
	// File writes
	regexp.MustCompile(`>>?\s`),    // redirect: echo > / tee >> etc.
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
