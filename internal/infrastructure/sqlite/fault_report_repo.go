package sqlite

import (
	"errors"
	"strings"

	"gorm.io/gorm"

	"github.com/ai-remote/workspace/internal/domain"
)

// FaultReportRepo persists distilled fault reports (host-attached incident
// assets) via GORM.
type FaultReportRepo struct {
	store *Store
}

// NewFaultReportRepo binds a FaultReportRepo to a Store.
func NewFaultReportRepo(store *Store) *FaultReportRepo {
	return &FaultReportRepo{store: store}
}

// Get returns one report by id.
func (r *FaultReportRepo) Get(id string) (domain.FaultReport, error) {
	var m faultReportModel
	if err := r.store.db.First(&m, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.FaultReport{}, gorm.ErrRecordNotFound
		}
		return domain.FaultReport{}, err
	}
	return faultReportFromModel(m), nil
}

// List returns reports matching the filter, newest update first.
func (r *FaultReportRepo) List(f domain.FaultReportFilter) ([]domain.FaultReport, error) {
	q := r.store.db.Model(&faultReportModel{})
	if f.HostID != "" {
		q = q.Where("host_id = ?", f.HostID)
	}
	if f.Severity != "" {
		q = q.Where("severity = ?", f.Severity)
	}
	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
	}
	if kw := strings.TrimSpace(f.Keyword); kw != "" {
		like := "%" + strings.ToLower(kw) + "%"
		q = q.Where("LOWER(title) LIKE ? OR LOWER(body) LIKE ?", like, like)
	}
	var models []faultReportModel
	if err := q.Order("updated_at DESC").Find(&models).Error; err != nil {
		return nil, err
	}
	out := make([]domain.FaultReport, 0, len(models))
	for _, m := range models {
		out = append(out, faultReportFromModel(m))
	}
	return out, nil
}

// Save inserts or updates one report.
func (r *FaultReportRepo) Save(rep domain.FaultReport) error {
	return r.store.db.Save(faultReportToModel(rep)).Error
}

// Delete removes one report by id.
func (r *FaultReportRepo) Delete(id string) error {
	return r.store.db.Delete(&faultReportModel{}, "id = ?", id).Error
}

func faultReportToModel(rep domain.FaultReport) faultReportModel {
	return faultReportModel{
		ID:             rep.ID,
		HostID:         rep.HostID,
		HostName:       rep.HostName,
		Title:          rep.Title,
		Severity:       rep.Severity,
		Status:         rep.Status,
		Body:           rep.Body,
		ConversationID: rep.ConversationID,
		CreatedAt:      rep.CreatedAt,
		UpdatedAt:      rep.UpdatedAt,
	}
}

func faultReportFromModel(m faultReportModel) domain.FaultReport {
	return domain.FaultReport{
		ID:             m.ID,
		HostID:         m.HostID,
		HostName:       m.HostName,
		Title:          m.Title,
		Severity:       m.Severity,
		Status:         m.Status,
		Body:           m.Body,
		ConversationID: m.ConversationID,
		CreatedAt:      m.CreatedAt,
		UpdatedAt:      m.UpdatedAt,
	}
}
