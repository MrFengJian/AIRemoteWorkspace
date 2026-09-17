package domain

import (
	"strings"
	"time"
)

// FaultReport is a traceable incident asset attached to a host: a structured
// diagnostic report distilled from an AI-assisted troubleshooting
// conversation. Reports accumulate per host so recurring problems can be
// tracked across sessions.
type FaultReport struct {
	ID string `json:"id"`
	// Host this report belongs to (the session's host at generation time).
	HostID   string `json:"hostId"`
	HostName string `json:"hostName"`
	Title    string `json:"title"`
	// Severity of the incident: SeverityInfo / SeverityWarning / SeverityCritical.
	Severity string `json:"severity"`
	// Tracking status: StatusOpen / StatusMonitoring / StatusResolved.
	Status string `json:"status"`
	// Body is the full report in markdown — 现象 / 根因 / 证据 / 处置 / 预防
	// sections per the SRE diagnosis output contract.
	Body string `json:"body"`
	// Conversation the report was distilled from (traceability back to the
	// full transcript in the agent history).
	ConversationID string `json:"conversationId,omitempty"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// Fault report severity levels.
const (
	SeverityInfo     = "info"
	SeverityWarning  = "warning"
	SeverityCritical = "critical"
)

// Fault report tracking statuses.
const (
	StatusOpen       = "open"
	StatusMonitoring = "monitoring"
	StatusResolved   = "resolved"
)

// FaultReportDraft is the LLM-distilled draft before the user reviews and
// saves it (host/conversation context is attached by the caller).
type FaultReportDraft struct {
	Title    string `json:"title"`
	Severity string `json:"severity"`
	Body     string `json:"body"`
}

// FaultReportFilter narrows fault-report listings; empty fields mean "no
// constraint". Keyword is matched case-insensitively against title and body.
type FaultReportFilter struct {
	HostID   string `json:"hostId"`
	Severity string `json:"severity"`
	Status   string `json:"status"`
	Keyword  string `json:"keyword"`
}

// NormalizeSeverity maps unknown values to SeverityWarning (the sensible
// middle for an unclassified incident).
func NormalizeSeverity(s string) string {
	switch strings.TrimSpace(strings.ToLower(s)) {
	case SeverityInfo:
		return SeverityInfo
	case SeverityCritical:
		return SeverityCritical
	default:
		return SeverityWarning
	}
}

// NormalizeStatus maps unknown values to StatusOpen.
func NormalizeStatus(s string) string {
	switch strings.TrimSpace(strings.ToLower(s)) {
	case StatusMonitoring:
		return StatusMonitoring
	case StatusResolved:
		return StatusResolved
	default:
		return StatusOpen
	}
}
