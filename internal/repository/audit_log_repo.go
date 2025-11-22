package repository

import (
	"github.com/google/uuid"
	"go-telegram-forwarder-bot/internal/models"
	"gorm.io/gorm"
)

type AuditLogRepository interface {
	Create(log *models.AuditLog) error
	GetByID(id uuid.UUID) (*models.AuditLog, error)
	GetByUserID(userID uuid.UUID, limit int) ([]*models.AuditLog, error)
	GetByActionType(actionType models.AuditLogAction, limit int) ([]*models.AuditLog, error)
	WithTx(tx *gorm.DB) AuditLogRepository
}

type auditLogRepository struct {
	db *gorm.DB
}

func NewAuditLogRepository(db *gorm.DB) AuditLogRepository {
	return &auditLogRepository{db: db}
}

func (r *auditLogRepository) Create(log *models.AuditLog) error {
	return r.db.Create(log).Error
}

func (r *auditLogRepository) GetByID(id uuid.UUID) (*models.AuditLog, error) {
	var log models.AuditLog
	if err := r.db.Preload("User").First(&log, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &log, nil
}

func (r *auditLogRepository) GetByUserID(userID uuid.UUID, limit int) ([]*models.AuditLog, error) {
	var logs []*models.AuditLog
	query := r.db.Where("user_id = ?", userID).Order("created_at DESC")
	if limit > 0 {
		query = query.Limit(limit)
	}
	if err := query.Preload("User").Find(&logs).Error; err != nil {
		return nil, err
	}
	return logs, nil
}

func (r *auditLogRepository) GetByActionType(actionType models.AuditLogAction, limit int) ([]*models.AuditLog, error) {
	var logs []*models.AuditLog
	query := r.db.Where("action_type = ?", actionType).Order("created_at DESC")
	if limit > 0 {
		query = query.Limit(limit)
	}
	if err := query.Preload("User").Find(&logs).Error; err != nil {
		return nil, err
	}
	return logs, nil
}

func (r *auditLogRepository) WithTx(tx *gorm.DB) AuditLogRepository {
	return &auditLogRepository{db: tx}
}
