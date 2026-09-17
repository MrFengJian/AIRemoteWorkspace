package application

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ai-remote/workspace/internal/domain"
)

// FaultReportRepository persists fault reports (implemented by the sqlite
// layer).
type FaultReportRepository interface {
	Get(id string) (domain.FaultReport, error)
	List(filter domain.FaultReportFilter) ([]domain.FaultReport, error)
	Save(rep domain.FaultReport) error
	Delete(id string) error
}

// FaultReportService manages host-attached incident reports: LLM-distilled
// from agent conversations, reviewed by the user, then tracked on the host
// (open → monitoring → resolved) as traceable assets.
type FaultReportService struct {
	repo FaultReportRepository
}

// NewFaultReportService wires the service over its repository.
func NewFaultReportService(repo FaultReportRepository) *FaultReportService {
	return &FaultReportService{repo: repo}
}

// List returns reports matching the filter, newest first.
func (s *FaultReportService) List(f domain.FaultReportFilter) ([]domain.FaultReport, error) {
	return s.repo.List(f)
}

// Get returns one report.
func (s *FaultReportService) Get(id string) (domain.FaultReport, error) {
	return s.repo.Get(id)
}

// Save creates or updates a report. Title is required; severity/status are
// normalized; on create the id and timestamps are stamped here.
func (s *FaultReportService) Save(rep domain.FaultReport) (domain.FaultReport, error) {
	rep.Title = strings.TrimSpace(rep.Title)
	if rep.Title == "" {
		return domain.FaultReport{}, errors.New("报告标题不能为空 / report title is required")
	}
	rep.Body = strings.TrimSpace(rep.Body)
	rep.Severity = domain.NormalizeSeverity(rep.Severity)
	rep.Status = domain.NormalizeStatus(rep.Status)
	if existing, err := s.repo.Get(rep.ID); err == nil {
		rep.CreatedAt = existing.CreatedAt
	} else {
		if rep.ID == "" {
			rep.ID = newID()
		}
		rep.CreatedAt = time.Now()
	}
	rep.UpdatedAt = time.Now()
	if err := s.repo.Save(rep); err != nil {
		return domain.FaultReport{}, fmt.Errorf("save fault report: %w", err)
	}
	return rep, nil
}

// SetStatus moves a report along its tracking lifecycle
// (open → monitoring → resolved; unknown values normalize to open).
func (s *FaultReportService) SetStatus(id, status string) (domain.FaultReport, error) {
	rep, err := s.repo.Get(id)
	if err != nil {
		return domain.FaultReport{}, err
	}
	rep.Status = domain.NormalizeStatus(status)
	rep.UpdatedAt = time.Now()
	if err := s.repo.Save(rep); err != nil {
		return domain.FaultReport{}, err
	}
	return rep, nil
}

// Delete removes one report.
func (s *FaultReportService) Delete(id string) error {
	return s.repo.Delete(id)
}

// ExtractFaultReportDraft parses the LLM's JSON answer (tolerating code
// fences or commentary around it) into a draft.
func ExtractFaultReportDraft(raw string) (domain.FaultReportDraft, error) {
	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start < 0 || end <= start {
		return domain.FaultReportDraft{}, errors.New("模型输出中没有 JSON 报告 / model returned no JSON report")
	}
	var d domain.FaultReportDraft
	if err := json.Unmarshal([]byte(raw[start:end+1]), &d); err != nil {
		return domain.FaultReportDraft{}, fmt.Errorf("解析故障报告草稿: %w", err)
	}
	d.Title = strings.TrimSpace(d.Title)
	d.Body = strings.TrimSpace(d.Body)
	d.Severity = domain.NormalizeSeverity(d.Severity)
	if d.Title == "" || d.Body == "" {
		return domain.FaultReportDraft{}, errors.New("故障报告草稿缺少标题或正文")
	}
	return d, nil
}
