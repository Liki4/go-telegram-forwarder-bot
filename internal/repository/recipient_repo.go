package repository

import (
	"github.com/google/uuid"
	"go-telegram-forwarder-bot/internal/models"
	"gorm.io/gorm"
)

type RecipientRepository interface {
	Create(recipient *models.Recipient) error
	GetByID(id uuid.UUID) (*models.Recipient, error)
	GetByBotID(botID uuid.UUID) ([]*models.Recipient, error)
	GetByBotIDAndChatID(botID uuid.UUID, chatID int64) (*models.Recipient, error)
	Update(recipient *models.Recipient) error
	Delete(id uuid.UUID) error
	DeleteByBotIDAndChatID(botID uuid.UUID, chatID int64) error
	WithTx(tx *gorm.DB) RecipientRepository
}

type recipientRepository struct {
	db *gorm.DB
}

func NewRecipientRepository(db *gorm.DB) RecipientRepository {
	return &recipientRepository{db: db}
}

func (r *recipientRepository) Create(recipient *models.Recipient) error {
	return r.db.Create(recipient).Error
}

func (r *recipientRepository) GetByID(id uuid.UUID) (*models.Recipient, error) {
	var recipient models.Recipient
	if err := r.db.First(&recipient, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &recipient, nil
}

func (r *recipientRepository) GetByBotID(botID uuid.UUID) ([]*models.Recipient, error) {
	var recipients []*models.Recipient
	if err := r.db.Where("bot_id = ?", botID).Find(&recipients).Error; err != nil {
		return nil, err
	}
	return recipients, nil
}

func (r *recipientRepository) GetByBotIDAndChatID(botID uuid.UUID, chatID int64) (*models.Recipient, error) {
	var recipient models.Recipient
	if err := r.db.Where("bot_id = ? AND chat_id = ?", botID, chatID).First(&recipient).Error; err != nil {
		return nil, err
	}
	return &recipient, nil
}

func (r *recipientRepository) Update(recipient *models.Recipient) error {
	return r.db.Save(recipient).Error
}

func (r *recipientRepository) Delete(id uuid.UUID) error {
	return r.db.Delete(&models.Recipient{}, "id = ?", id).Error
}

func (r *recipientRepository) DeleteByBotIDAndChatID(botID uuid.UUID, chatID int64) error {
	return r.db.Where("bot_id = ? AND chat_id = ?", botID, chatID).Delete(&models.Recipient{}).Error
}

func (r *recipientRepository) WithTx(tx *gorm.DB) RecipientRepository {
	return &recipientRepository{db: tx}
}
