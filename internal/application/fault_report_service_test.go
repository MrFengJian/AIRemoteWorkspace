package application

import (
	"errors"
	"strings"
	"testing"

	"github.com/ai-remote/workspace/internal/domain"
)

// memFaultRepo is an in-memory FaultReportRepository for tests.
type memFaultRepo struct {
	items map[string]domain.FaultReport
}

func newMemFaultRepo() *memFaultRepo {
	return &memFaultRepo{items: make(map[string]domain.FaultReport)}
}

func (r *memFaultRepo) Get(id string) (domain.FaultReport, error) {
	rep, ok := r.items[id]
	if !ok {
		return domain.FaultReport{}, errNotFound
	}
	return rep, nil
}

func (r *memFaultRepo) List(f domain.FaultReportFilter) ([]domain.FaultReport, error) {
	var out []domain.FaultReport
	kw := strings.ToLower(f.Keyword)
	for _, rep := range r.items {
		if f.HostID != "" && rep.HostID != f.HostID {
			continue
		}
		if f.Severity != "" && rep.Severity != f.Severity {
			continue
		}
		if f.Status != "" && rep.Status != f.Status {
			continue
		}
		if kw != "" &&
			!strings.Contains(strings.ToLower(rep.Title), kw) &&
			!strings.Contains(strings.ToLower(rep.Body), kw) {
			continue
		}
		out = append(out, rep)
	}
	return out, nil
}

func (r *memFaultRepo) Save(rep domain.FaultReport) error {
	r.items[rep.ID] = rep
	return nil
}

func (r *memFaultRepo) Delete(id string) error {
	delete(r.items, id)
	return nil
}

var errNotFound = errors.New("not found")

// TestFaultReportServiceTracking covers create → filter → status lifecycle
// → delete, plus normalization of unknown severity/status values.
func TestFaultReportServiceTracking(t *testing.T) {
	svc := NewFaultReportService(newMemFaultRepo())

	saved, err := svc.Save(domain.FaultReport{
		HostID: "host-1", HostName: "web-01", Title: "Nginx 502 上游超时",
		Severity: "bogus", Status: "bogus", Body: "## 现象\n502",
	})
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if saved.ID == "" {
		t.Fatal("create must stamp an id")
	}
	if saved.Severity != domain.SeverityWarning || saved.Status != domain.StatusOpen {
		t.Fatalf("unknown values must normalize, got severity=%q status=%q", saved.Severity, saved.Status)
	}
	if saved.CreatedAt.IsZero() || saved.UpdatedAt.IsZero() {
		t.Fatal("create must stamp timestamps")
	}

	// Status lifecycle.
	moved, err := svc.SetStatus(saved.ID, domain.StatusMonitoring)
	if err != nil {
		t.Fatalf("set status: %v", err)
	}
	if moved.Status != domain.StatusMonitoring || moved.UpdatedAt.Before(moved.CreatedAt) {
		t.Fatalf("status move wrong: %+v", moved)
	}

	// Filters.
	if _, err := svc.Save(domain.FaultReport{HostID: "host-2", HostName: "db-01", Title: "MySQL 连接暴涨"}); err != nil {
		t.Fatal(err)
	}
	all, err := svc.List(domain.FaultReportFilter{})
	if err != nil || len(all) != 2 {
		t.Fatalf("list all: %v len=%d", err, len(all))
	}
	byHost, _ := svc.List(domain.FaultReportFilter{HostID: "host-1"})
	if len(byHost) != 1 || byHost[0].Title != "Nginx 502 上游超时" {
		t.Fatalf("host filter wrong: %+v", byHost)
	}
	byKW, _ := svc.List(domain.FaultReportFilter{Keyword: "mysql"})
	if len(byKW) != 1 || byKW[0].HostName != "db-01" {
		t.Fatalf("keyword filter wrong: %+v", byKW)
	}
	byStatus, _ := svc.List(domain.FaultReportFilter{Status: domain.StatusOpen})
	if len(byStatus) != 1 {
		t.Fatalf("status filter wrong: %+v", byStatus)
	}

	// Edit keeps CreatedAt.
	again, err := svc.Save(domain.FaultReport{
		ID: saved.ID, HostID: "host-1", HostName: "web-01",
		Title: "Nginx 502（已恢复）", Severity: domain.SeverityInfo,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !again.CreatedAt.Equal(saved.CreatedAt) {
		t.Fatal("update must keep CreatedAt")
	}

	// Title required.
	if _, err := svc.Save(domain.FaultReport{Title: "  "}); err == nil {
		t.Fatal("empty title must be rejected")
	}

	// Delete.
	if err := svc.Delete(saved.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Get(saved.ID); err == nil {
		t.Fatal("deleted report still readable")
	}
}

// TestExtractFaultReportDraft parses the model's JSON answer tolerating
// fences/commentary, and normalizes severity.
func TestExtractFaultReportDraft(t *testing.T) {
	raw := "好的，以下是报告：\n```json\n" +
		`{"title":"Redis 连接拒绝","severity":"critical","body":"## 现象\n连接被拒"}` +
		"\n```"
	d, err := ExtractFaultReportDraft(raw)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if d.Title != "Redis 连接拒绝" || d.Severity != domain.SeverityCritical ||
		!strings.Contains(d.Body, "## 现象") {
		t.Fatalf("draft wrong: %+v", d)
	}

	// Unknown severity normalizes to warning; missing body rejected.
	d, err = ExtractFaultReportDraft(`{"title":"x","severity":"?","body":"b"}`)
	if err != nil || d.Severity != domain.SeverityWarning {
		t.Fatalf("normalize wrong: %+v err=%v", d, err)
	}
	if _, err := ExtractFaultReportDraft(`{"title":"x","body":""}`); err == nil {
		t.Fatal("empty body must be rejected")
	}
	if _, err := ExtractFaultReportDraft("no json here"); err == nil {
		t.Fatal("non-JSON output must be rejected")
	}
}
