package repository

import (
	"github.com/google/uuid"
	"go-telegram-forwarder-bot/internal/models"
	"gorm.io/gorm"
)

type BotAdminRepository interface {
	Create(admin *models.BotAdmin) error
	GetByID(id uuid.UUID) (*models.BotAdmin, error)
	GetByBotID(botID uuid.UUID) ([]*models.BotAdmin, error)
	GetByBotIDAndUserID(botID uuid.UUID, userID uuid.UUID) (*models.BotAdmin, error)
	IsAdmin(botID uuid.UUID, userID uuid.UUID) (bool, error)
	Delete(id uuid.UUID) error
	DeleteByBotIDAndUserID(botID uuid.UUID, userID uuid.UUID) error
}

type botAdminRepository struct {
	db *gorm.DB
}

func NewBotAdminRepository(db *gorm.DB) BotAdminRepository {
	return &botAdminRepository{db: db}
}

func (r *botAdminRepository) Create(admin *models.BotAdmin) error {
	return r.db.Create(admin).Error
}

func (r *botAdminRepository) GetByID(id uuid.UUID) (*models.BotAdmin, error) {
	var admin models.BotAdmin
	if err := r.db.Preload("AdminUser").First(&admin, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &admin, nil
}

func (r *botAdminRepository) GetByBotID(botID uuid.UUID) ([]*models.BotAdmin, error) {
	var admins []*models.BotAdmin
	if err := r.db.Where("bot_id = ?", botID).
		Preload("AdminUser").Find(&admins).Error; err != nil {
		return nil, err
	}
	return admins, nil
}

func (r *botAdminRepository) GetByBotIDAndUserID(botID uuid.UUID, userID uuid.UUID) (*models.BotAdmin, error) {
	var admin models.BotAdmin
	if err := r.db.Where("bot_id = ? AND admin_user_id = ?", botID, userID).
		First(&admin).Error; err != nil {
		return nil, err
	}
	return &admin, nil
}

func (r *botAdminRepository) IsAdmin(botID uuid.UUID, userID uuid.UUID) (bool, error) {
	var count int64
	if err := r.db.Model(&models.BotAdmin{}).
		Where("bot_id = ? AND admin_user_id = ? AND deleted_at IS NULL", botID, userID).
		Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *botAdminRepository) Delete(id uuid.UUID) error {
	return r.db.Delete(&models.BotAdmin{}, "id = ?", id).Error
}

func (r *botAdminRepository) DeleteByBotIDAndUserID(botID uuid.UUID, userID uuid.UUID) error {
	return r.db.Where("bot_id = ? AND admin_user_id = ?", botID, userID).
		Delete(&models.BotAdmin{}).Error
}
