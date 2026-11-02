package sqlite

import (
	"encoding/json"
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/ai-remote/workspace/internal/domain"
)

// ── 应用部署持久化模型（安装记录 / 部署任务 / 目录元数据）────────────────

// appInstallationModel 镜像 app_installations 表。
type appInstallationModel struct {
	ID          string `gorm:"primaryKey;size:64"`
	HostID      string `gorm:"not null;size:64;uniqueIndex:idx_app_install_host_app"`
	AppID       string `gorm:"not null;size:64;uniqueIndex:idx_app_install_host_app"`
	Status      string `gorm:"not null;size:20;index"`
	AppVersion  string `gorm:"not null;default:'';size:100"`
	ServiceUnit string `gorm:"not null;default:'';size:100"`
	Port        int    `gorm:"not null;default:0"`
	InstallDir  string `gorm:"not null;default:'';size:500"`
	LastError   string `gorm:"not null;default:'';type:text"`
	InstalledAt time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (appInstallationModel) TableName() string { return "app_installations" }

// appDeployTaskModel 镜像 app_deploy_tasks 表（步骤 JSON 内嵌）。
type appDeployTaskModel struct {
	ID         string `gorm:"primaryKey;size:64"`
	HostID     string `gorm:"not null;size:64;index"`
	SessionID  string `gorm:"not null;default:'';size:64"`
	AppID      string `gorm:"not null;size:64;index"`
	Kind       string `gorm:"not null;size:20"`
	Status     string `gorm:"not null;size:20;index"`
	StepsJSON  string `gorm:"not null;type:text"`
	StartedAt  time.Time
	FinishedAt *time.Time
	LastError  string `gorm:"not null;default:'';type:text"`
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

func (appDeployTaskModel) TableName() string { return "app_deploy_tasks" }

// appCatalogMetaModel 镜像 app_catalog_meta 表（单行）。
type appCatalogMetaModel struct {
	ID       int `gorm:"primaryKey"` // 恒为 1
	Source   string `gorm:"not null;size:20"`
	Repo     string `gorm:"not null;default:'';size:200"`
	Version  int    `gorm:"not null;default:0"`
	Ref      string `gorm:"not null;default:'';size:100"`
	SyncedAt time.Time
}

func (appCatalogMetaModel) TableName() string { return "app_catalog_meta" }

// ── 领域 ↔ 模型转换 ──────────────────────────────────────────────────────

func installationToModel(in domain.Installation) appInstallationModel {
	return appInstallationModel{
		ID: installationID(in.HostID, in.AppID), HostID: in.HostID, AppID: in.AppID,
		Status: in.Status, AppVersion: in.AppVersion, ServiceUnit: in.ServiceUnit,
		Port: in.Port, InstallDir: in.InstallDir, LastError: in.LastError,
		InstalledAt: in.InstalledAt, CreatedAt: in.InstalledAt, UpdatedAt: in.UpdatedAt,
	}
}

func installationFromModel(m appInstallationModel) domain.Installation {
	return domain.Installation{
		ID: m.ID, HostID: m.HostID, AppID: m.AppID, Status: m.Status,
		AppVersion: m.AppVersion, ServiceUnit: m.ServiceUnit, Port: m.Port,
		InstallDir: m.InstallDir, LastError: m.LastError,
		InstalledAt: m.InstalledAt, UpdatedAt: m.UpdatedAt,
	}
}

// installationID 以 主机+应用 为主键（一台主机一个应用一条记录）。
func installationID(hostID, appID string) string { return hostID + ":" + appID }

func taskToModel(t domain.DeployTask) (appDeployTaskModel, error) {
	steps, err := json.Marshal(t.Steps)
	if err != nil {
		return appDeployTaskModel{}, err
	}
	return appDeployTaskModel{
		ID: t.ID, HostID: t.HostID, SessionID: t.SessionID, AppID: t.AppID,
		Kind: t.Kind, Status: t.Status, StepsJSON: string(steps),
		StartedAt: t.StartedAt, FinishedAt: t.FinishedAt, LastError: t.LastError,
	}, nil
}

func taskFromModel(m appDeployTaskModel) (domain.DeployTask, error) {
	var steps []domain.DeployStepRun
	if m.StepsJSON != "" {
		if err := json.Unmarshal([]byte(m.StepsJSON), &steps); err != nil {
			return domain.DeployTask{}, err
		}
	}
	return domain.DeployTask{
		ID: m.ID, HostID: m.HostID, SessionID: m.SessionID, AppID: m.AppID,
		Kind: m.Kind, Status: m.Status, Steps: steps,
		StartedAt: m.StartedAt, FinishedAt: m.FinishedAt, LastError: m.LastError,
	}, nil
}

// ── 仓库实现 ─────────────────────────────────────────────────────────────

// AppDeployRepo 持久化应用部署相关数据（安装记录/任务/目录元数据）。
type AppDeployRepo struct {
	store *Store
}

// NewAppDeployRepo binds an AppDeployRepo to a Store.
func NewAppDeployRepo(store *Store) *AppDeployRepo { return &AppDeployRepo{store: store} }

// SaveInstallation 插入或更新一条安装记录（主机+应用 唯一）。
func (r *AppDeployRepo) SaveInstallation(inst domain.Installation) error {
	m := installationToModel(inst)
	if m.UpdatedAt.IsZero() {
		m.UpdatedAt = time.Now()
	}
	return r.store.db.Save(&m).Error
}

// GetInstallation 返回一条安装记录；不存在返回 gorm.ErrRecordNotFound。
func (r *AppDeployRepo) GetInstallation(hostID, appID string) (domain.Installation, error) {
	var m appInstallationModel
	if err := r.store.db.First(&m, "id = ?", installationID(hostID, appID)).Error; err != nil {
		return domain.Installation{}, err
	}
	return installationFromModel(m), nil
}

// ListInstallations 返回安装记录（hostID 为空 = 全部），按更新时间倒序。
func (r *AppDeployRepo) ListInstallations(hostID string) ([]domain.Installation, error) {
	q := r.store.db.Model(&appInstallationModel{})
	if hostID != "" {
		q = q.Where("host_id = ?", hostID)
	}
	var models []appInstallationModel
	if err := q.Order("updated_at DESC").Find(&models).Error; err != nil {
		return nil, err
	}
	out := make([]domain.Installation, 0, len(models))
	for _, m := range models {
		out = append(out, installationFromModel(m))
	}
	return out, nil
}

// DeleteInstallation 移除一条安装记录（不存在不报错）。
func (r *AppDeployRepo) DeleteInstallation(hostID, appID string) error {
	return r.store.db.Delete(&appInstallationModel{}, "id = ?", installationID(hostID, appID)).Error
}

// SaveTask 插入或更新一个部署任务（含步骤 JSON）。
func (r *AppDeployRepo) SaveTask(t domain.DeployTask) error {
	m, err := taskToModel(t)
	if err != nil {
		return err
	}
	if m.CreatedAt.IsZero() {
		m.CreatedAt = time.Now()
	}
	// Save 需要 CreatedAt 非零才不会重置；已存在记录改为 Updates 保留原值。
	var count int64
	if err := r.store.db.Model(&appDeployTaskModel{}).Where("id = ?", m.ID).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return r.store.db.Model(&appDeployTaskModel{}).Where("id = ?", m.ID).Updates(map[string]any{
			"status": m.Status, "steps_json": m.StepsJSON, "finished_at": m.FinishedAt,
			"last_error": m.LastError, "updated_at": time.Now(),
		}).Error
	}
	return r.store.db.Create(&m).Error
}

// GetTask 返回一个部署任务；不存在返回 gorm.ErrRecordNotFound。
func (r *AppDeployRepo) GetTask(id string) (domain.DeployTask, error) {
	var m appDeployTaskModel
	if err := r.store.db.First(&m, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.DeployTask{}, err
		}
		return domain.DeployTask{}, err
	}
	return taskFromModel(m)
}

// ListTasks 返回任务（hostID 为空 = 全部），按开始时间倒序，limit 截断。
func (r *AppDeployRepo) ListTasks(hostID string, limit int) ([]domain.DeployTask, error) {
	q := r.store.db.Model(&appDeployTaskModel{})
	if hostID != "" {
		q = q.Where("host_id = ?", hostID)
	}
	var models []appDeployTaskModel
	if err := q.Order("started_at DESC").Limit(limit).Find(&models).Error; err != nil {
		return nil, err
	}
	out := make([]domain.DeployTask, 0, len(models))
	for _, m := range models {
		t, err := taskFromModel(m)
		if err != nil {
			continue // 单条损坏跳过
		}
		out = append(out, t)
	}
	return out, nil
}

// SaveCatalogMeta 写入目录元数据（单行 upsert）。
func (r *AppDeployRepo) SaveCatalogMeta(m domain.CatalogMeta) error {
	model := appCatalogMetaModel{ID: 1, Source: m.Source, Repo: m.Repo, Version: m.Version, Ref: m.Ref, SyncedAt: m.SyncedAt}
	return r.store.db.Save(&model).Error
}

// GetCatalogMeta 返回目录元数据；从未同步过返回零值（不视为错误）。
func (r *AppDeployRepo) GetCatalogMeta() (domain.CatalogMeta, error) {
	var m appCatalogMetaModel
	err := r.store.db.First(&m, "id = 1").Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.CatalogMeta{}, nil
	}
	if err != nil {
		return domain.CatalogMeta{}, err
	}
	return domain.CatalogMeta{Source: m.Source, Repo: m.Repo, Version: m.Version, Ref: m.Ref, SyncedAt: m.SyncedAt}, nil
}
