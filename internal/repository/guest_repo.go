package repository

import (
	"github.com/google/uuid"
	"go-telegram-forwarder-bot/internal/models"
	"gorm.io/gorm"
)

type GuestRepository interface {
	Create(guest *models.Guest) error
	GetByID(id uuid.UUID) (*models.Guest, error)
	GetByBotID(botID uuid.UUID) ([]*models.Guest, error)
	GetByBotIDAndUserID(botID uuid.UUID, userID int64) (*models.Guest, error)
	GetOrCreateByBotIDAndUserID(botID uuid.UUID, userID int64) (*models.Guest, error)
	CountByBotID(botID uuid.UUID) (int64, error)
	Delete(id uuid.UUID) error
}

type guestRepository struct {
	db *gorm.DB
}

func NewGuestRepository(db *gorm.DB) GuestRepository {
	return &guestRepository{db: db}
}

func (r *guestRepository) Create(guest *models.Guest) error {
	return r.db.Create(guest).Error
}

func (r *guestRepository) GetByID(id uuid.UUID) (*models.Guest, error) {
	var guest models.Guest
	if err := r.db.First(&guest, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &guest, nil
}

func (r *guestRepository) GetByBotID(botID uuid.UUID) ([]*models.Guest, error) {
	var guests []*models.Guest
	if err := r.db.Where("bot_id = ?", botID).Find(&guests).Error; err != nil {
		return nil, err
	}
	return guests, nil
}

func (r *guestRepository) GetByBotIDAndUserID(botID uuid.UUID, userID int64) (*models.Guest, error) {
	var guest models.Guest
	if err := r.db.Where("bot_id = ? AND guest_user_id = ?", botID, userID).First(&guest).Error; err != nil {
		return nil, err
	}
	return &guest, nil
}

func (r *guestRepository) GetOrCreateByBotIDAndUserID(botID uuid.UUID, userID int64) (*models.Guest, error) {
	guest, err := r.GetByBotIDAndUserID(botID, userID)
	if err == nil {
		return guest, nil
	}

	if err != gorm.ErrRecordNotFound {
		return nil, err
	}

	newGuest := &models.Guest{
		BotID:       botID,
		GuestUserID: userID,
	}
	if err := r.Create(newGuest); err != nil {
		return nil, err
	}
	return newGuest, nil
}

func (r *guestRepository) CountByBotID(botID uuid.UUID) (int64, error) {
	var count int64
	if err := r.db.Model(&models.Guest{}).Where("bot_id = ?", botID).Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func (r *guestRepository) Delete(id uuid.UUID) error {
	return r.db.Delete(&models.Guest{}, "id = ?", id).Error
}
