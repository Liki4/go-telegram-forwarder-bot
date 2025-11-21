package repository

import (
	"github.com/google/uuid"
	"go-telegram-forwarder-bot/internal/models"
	"gorm.io/gorm"
)

type BlacklistApprovalMessageRepository interface {
	Create(msg *models.BlacklistApprovalMessage) error
	GetByBlacklistID(blacklistID uuid.UUID) ([]*models.BlacklistApprovalMessage, error)
	GetByBlacklistIDAndUserID(blacklistID uuid.UUID, userID uuid.UUID) (*models.BlacklistApprovalMessage, error)
	DeleteByBlacklistID(blacklistID uuid.UUID) error
}

type blacklistApprovalMessageRepository struct {
	db *gorm.DB
}

func NewBlacklistApprovalMessageRepository(db *gorm.DB) BlacklistApprovalMessageRepository {
	return &blacklistApprovalMessageRepository{db: db}
}

func (r *blacklistApprovalMessageRepository) Create(msg *models.BlacklistApprovalMessage) error {
	return r.db.Create(msg).Error
}

func (r *blacklistApprovalMessageRepository) GetByBlacklistID(blacklistID uuid.UUID) ([]*models.BlacklistApprovalMessage, error) {
	var messages []*models.BlacklistApprovalMessage
	if err := r.db.Where("blacklist_id = ? AND deleted_at IS NULL", blacklistID).
		Preload("User").Find(&messages).Error; err != nil {
		return nil, err
	}
	return messages, nil
}

func (r *blacklistApprovalMessageRepository) GetByBlacklistIDAndUserID(blacklistID uuid.UUID, userID uuid.UUID) (*models.BlacklistApprovalMessage, error) {
	var msg models.BlacklistApprovalMessage
	if err := r.db.Where("blacklist_id = ? AND user_id = ? AND deleted_at IS NULL", blacklistID, userID).
		First(&msg).Error; err != nil {
		return nil, err
	}
	return &msg, nil
}

func (r *blacklistApprovalMessageRepository) DeleteByBlacklistID(blacklistID uuid.UUID) error {
	return r.db.Where("blacklist_id = ?", blacklistID).
		Delete(&models.BlacklistApprovalMessage{}).Error
}

