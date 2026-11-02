package appdeploy

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/ai-remote/workspace/internal/domain"
)

// ParseHostAppStatus 解析 ops.status 输出的 KEY=VALUE 契约（见
// docs/APP_CATALOG.md）：STATUS/PID/PORT/LISTENING/UPTIME_SEC/MEM_KB/
// HEALTH/VERSION。字段缺省时取零值/unknown，绝不臆测。
func ParseHostAppStatus(raw string) domain.HostAppStatus {
	st := domain.HostAppStatus{State: domain.AppStateUnknown, Health: "unknown", Raw: raw}
	for _, line := range strings.Split(raw, "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		switch key {
		case "STATUS":
			switch value {
			case domain.AppStateRunning, domain.AppStateStopped, domain.AppStateFailed:
				st.State = value
			default:
				st.State = domain.AppStateUnknown
			}
		case "PID":
			st.PID = atoiOrZero(value)
		case "PORT":
			st.Port = atoiOrZero(value)
		case "LISTENING":
			st.Listening = value == "yes" || value == "true" || value == "1"
		case "UPTIME_SEC":
			st.UptimeSec = int64(atoiOrZero(value))
		case "MEM_KB":
			st.MemKB = int64(atoiOrZero(value))
		case "HEALTH":
			st.Health = value
		case "VERSION":
			st.Version = value
		}
	}
	return st
}

func atoiOrZero(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return n
}

// FormatUptime 把秒格式化为 "1-16:54:39"（天-时:分:秒）与人类可读内存，
// 供前端直接展示（逻辑放在后端保证两端一致）。
func FormatUptime(sec int64) string {
	if sec <= 0 {
		return "-"
	}
	d := sec / 86400
	h := (sec % 86400) / 3600
	m := (sec % 3600) / 60
	s := sec % 60
	if d > 0 {
		return fmt.Sprintf("%d-%d:%02d:%02d", d, h, m, s)
	}
	return fmt.Sprintf("%d:%02d:%02d", h, m, s)
}
