package repository

import (
	"github.com/google/uuid"
	"go-telegram-forwarder-bot/internal/models"
	"gorm.io/gorm"
	"time"
)

type BlacklistRepository interface {
	Create(blacklist *models.Blacklist) error
	GetByID(id uuid.UUID) (*models.Blacklist, error)
	GetByBotIDAndGuestID(botID uuid.UUID, guestID uuid.UUID) (*models.Blacklist, error)
	GetAllByBotIDAndGuestID(botID uuid.UUID, guestID uuid.UUID) ([]*models.Blacklist, error)
	GetActiveByBotIDAndGuestID(botID uuid.UUID, guestID uuid.UUID) (*models.Blacklist, error)
	GetPendingByBotID(botID uuid.UUID) ([]*models.Blacklist, error)
	GetPendingOrApprovedBanByBotIDAndGuestID(botID uuid.UUID, guestID uuid.UUID) (*models.Blacklist, error)
	GetLatestApprovedUnbanByBotIDAndGuestID(botID uuid.UUID, guestID uuid.UUID) (*models.Blacklist, error)
	GetLatestByBotIDAndGuestID(botID uuid.UUID, guestID uuid.UUID) (*models.Blacklist, error)
	Update(blacklist *models.Blacklist) error
	ApprovePending(id uuid.UUID) error
	RejectPending(id uuid.UUID) error
	AutoApproveExpired() error
}

type blacklistRepository struct {
	db *gorm.DB
}

func NewBlacklistRepository(db *gorm.DB) BlacklistRepository {
	return &blacklistRepository{db: db}
}

func (r *blacklistRepository) Create(blacklist *models.Blacklist) error {
	return r.db.Create(blacklist).Error
}

func (r *blacklistRepository) GetByID(id uuid.UUID) (*models.Blacklist, error) {
	var blacklist models.Blacklist
	if err := r.db.Preload("Guest").Preload("RequestUser").First(&blacklist, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &blacklist, nil
}

func (r *blacklistRepository) GetByBotIDAndGuestID(botID uuid.UUID, guestID uuid.UUID) (*models.Blacklist, error) {
	var blacklist models.Blacklist
	if err := r.db.Where("bot_id = ? AND guest_id = ?", botID, guestID).
		Order("created_at DESC").First(&blacklist).Error; err != nil {
		return nil, err
	}
	return &blacklist, nil
}

func (r *blacklistRepository) GetAllByBotIDAndGuestID(botID uuid.UUID, guestID uuid.UUID) ([]*models.Blacklist, error) {
	var blacklists []*models.Blacklist
	if err := r.db.Where("bot_id = ? AND guest_id = ? AND deleted_at IS NULL", botID, guestID).
		Order("created_at DESC").Find(&blacklists).Error; err != nil {
		return nil, err
	}
	return blacklists, nil
}

func (r *blacklistRepository) GetActiveByBotIDAndGuestID(botID uuid.UUID, guestID uuid.UUID) (*models.Blacklist, error) {
	var blacklist models.Blacklist
	if err := r.db.Where("bot_id = ? AND guest_id = ? AND status = ? AND deleted_at IS NULL",
		botID, guestID, models.BlacklistStatusApproved).
		Order("created_at DESC").First(&blacklist).Error; err != nil {
		return nil, err
	}
	return &blacklist, nil
}

func (r *blacklistRepository) GetPendingOrApprovedBanByBotIDAndGuestID(botID uuid.UUID, guestID uuid.UUID) (*models.Blacklist, error) {
	var blacklist models.Blacklist
	if err := r.db.Where("bot_id = ? AND guest_id = ? AND request_type = ? AND status IN ? AND deleted_at IS NULL",
		botID, guestID, models.BlacklistRequestTypeBan, []models.BlacklistStatus{models.BlacklistStatusPending, models.BlacklistStatusApproved}).
		Order("created_at DESC").First(&blacklist).Error; err != nil {
		return nil, err
	}
	return &blacklist, nil
}

func (r *blacklistRepository) GetLatestApprovedUnbanByBotIDAndGuestID(botID uuid.UUID, guestID uuid.UUID) (*models.Blacklist, error) {
	var blacklist models.Blacklist
	if err := r.db.Where("bot_id = ? AND guest_id = ? AND request_type = ? AND status = ? AND deleted_at IS NULL",
		botID, guestID, models.BlacklistRequestTypeUnban, models.BlacklistStatusApproved).
		Order("created_at DESC").First(&blacklist).Error; err != nil {
		return nil, err
	}
	return &blacklist, nil
}

// GetLatestByBotIDAndGuestID gets the latest blacklist record for a guest (regardless of type or status)
// This is optimized to only fetch the most recent record instead of all records
func (r *blacklistRepository) GetLatestByBotIDAndGuestID(botID uuid.UUID, guestID uuid.UUID) (*models.Blacklist, error) {
	var blacklist models.Blacklist
	if err := r.db.Where("bot_id = ? AND guest_id = ? AND deleted_at IS NULL",
		botID, guestID).
		Order("created_at DESC").First(&blacklist).Error; err != nil {
		return nil, err
	}
	return &blacklist, nil
}

func (r *blacklistRepository) GetPendingByBotID(botID uuid.UUID) ([]*models.Blacklist, error) {
	var blacklists []*models.Blacklist
	if err := r.db.Where("bot_id = ? AND status = ?", botID, models.BlacklistStatusPending).
		Preload("Guest").Preload("RequestUser").Find(&blacklists).Error; err != nil {
		return nil, err
	}
	return blacklists, nil
}

func (r *blacklistRepository) Update(blacklist *models.Blacklist) error {
	return r.db.Save(blacklist).Error
}

func (r *blacklistRepository) ApprovePending(id uuid.UUID) error {
	now := time.Now()
	return r.db.Model(&models.Blacklist{}).
		Where("id = ? AND status = ?", id, models.BlacklistStatusPending).
		Updates(map[string]interface{}{
			"status":      models.BlacklistStatusApproved,
			"approved_at": &now,
		}).Error
}

func (r *blacklistRepository) RejectPending(id uuid.UUID) error {
	return r.db.Model(&models.Blacklist{}).
		Where("id = ? AND status = ?", id, models.BlacklistStatusPending).
		Update("status", models.BlacklistStatusRejected).Error
}

func (r *blacklistRepository) AutoApproveExpired() error {
	oneDayAgo := time.Now().Add(-24 * time.Hour)
	now := time.Now()
	return r.db.Model(&models.Blacklist{}).
		Where("status = ? AND created_at < ?", models.BlacklistStatusPending, oneDayAgo).
		Updates(map[string]interface{}{
			"status":      models.BlacklistStatusApproved,
			"approved_at": &now,
		}).Error
}
