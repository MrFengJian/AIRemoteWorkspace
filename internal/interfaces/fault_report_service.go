package interfaces

import (
	"context"
	"fmt"
	"time"

	wailsapp "github.com/wailsapp/wails/v3/pkg/application"

	appsvc "github.com/ai-remote/workspace/internal/application"
	"github.com/ai-remote/workspace/internal/domain"
)

// FaultReportDTO mirrors domain.FaultReport for the frontend: host-attached
// incident reports tracked on the 故障报告 page.
type FaultReportDTO struct {
	ID             string `json:"id"`
	HostID         string `json:"hostId"`
	HostName       string `json:"hostName"`
	Title          string `json:"title"`
	Severity       string `json:"severity"`
	Status         string `json:"status"`
	Body           string `json:"body"`
	ConversationID string `json:"conversationId,omitempty"`
	CreatedAt      string `json:"createdAt"`
	UpdatedAt      string `json:"updatedAt"`
}

// FaultReportFilterDTO narrows ListReports; empty fields = no constraint.
type FaultReportFilterDTO struct {
	HostID   string `json:"hostId"`
	Severity string `json:"severity"`
	Status   string `json:"status"`
	Keyword  string `json:"keyword"`
}

func faultReportToDTO(r domain.FaultReport) FaultReportDTO {
	return FaultReportDTO{
		ID:             r.ID,
		HostID:         r.HostID,
		HostName:       r.HostName,
		Title:          r.Title,
		Severity:       r.Severity,
		Status:         r.Status,
		Body:           r.Body,
		ConversationID: r.ConversationID,
		CreatedAt:      r.CreatedAt.Format(time.RFC3339),
		UpdatedAt:      r.UpdatedAt.Format(time.RFC3339),
	}
}

// FaultReportService exposes the host-attached incident reports to the
// frontend: list + filter on the 故障报告 page, save from the agent's
// report-distillation dialog, status tracking and delete.
type FaultReportService struct {
	reports *appsvc.FaultReportService
}

// NewFaultReportService wires the Wails FaultReportService.
func NewFaultReportService(reports *appsvc.FaultReportService) *FaultReportService {
	return &FaultReportService{reports: reports}
}

func (s *FaultReportService) ServiceName() string { return "FaultReportService" }

func (s *FaultReportService) ServiceStartup(_ context.Context, _ wailsapp.ServiceOptions) error {
	return nil
}

// ListReports returns reports matching the filter (newest first).
func (s *FaultReportService) ListReports(filter FaultReportFilterDTO) ([]FaultReportDTO, error) {
	if s.reports == nil {
		return []FaultReportDTO{}, nil
	}
	list, err := s.reports.List(domain.FaultReportFilter{
		HostID:   filter.HostID,
		Severity: filter.Severity,
		Status:   filter.Status,
		Keyword:  filter.Keyword,
	})
	if err != nil {
		return nil, err
	}
	out := make([]FaultReportDTO, 0, len(list))
	for _, r := range list {
		out = append(out, faultReportToDTO(r))
	}
	return out, nil
}

// GetReport returns one report with its full markdown body.
func (s *FaultReportService) GetReport(id string) (FaultReportDTO, error) {
	if s.reports == nil {
		return FaultReportDTO{}, fmt.Errorf("fault reports not available")
	}
	r, err := s.reports.Get(id)
	if err != nil {
		return FaultReportDTO{}, err
	}
	return faultReportToDTO(r), nil
}

// SaveReport creates or updates a report (the frontend sends the reviewed
// draft with host context; severity/status normalize server-side).
func (s *FaultReportService) SaveReport(dto FaultReportDTO) (FaultReportDTO, error) {
	if s.reports == nil {
		return FaultReportDTO{}, fmt.Errorf("fault reports not available")
	}
	saved, err := s.reports.Save(domain.FaultReport{
		ID:             dto.ID,
		HostID:         dto.HostID,
		HostName:       dto.HostName,
		Title:          dto.Title,
		Severity:       dto.Severity,
		Status:         dto.Status,
		Body:           dto.Body,
		ConversationID: dto.ConversationID,
	})
	if err != nil {
		return FaultReportDTO{}, err
	}
	return faultReportToDTO(saved), nil
}

// SetReportStatus moves a report along its tracking lifecycle
// (open → monitoring → resolved).
func (s *FaultReportService) SetReportStatus(id, status string) (FaultReportDTO, error) {
	if s.reports == nil {
		return FaultReportDTO{}, fmt.Errorf("fault reports not available")
	}
	r, err := s.reports.SetStatus(id, status)
	if err != nil {
		return FaultReportDTO{}, err
	}
	return faultReportToDTO(r), nil
}

// DeleteReport removes one report.
func (s *FaultReportService) DeleteReport(id string) error {
	if s.reports == nil {
		return fmt.Errorf("fault reports not available")
	}
	return s.reports.Delete(id)
}
