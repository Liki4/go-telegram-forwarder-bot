package repository

import (
	"github.com/google/uuid"
	"go-telegram-forwarder-bot/internal/models"
	"gorm.io/gorm"
)

type MessageMappingRepository interface {
	Create(mapping *models.MessageMapping) error
	GetByID(id uuid.UUID) (*models.MessageMapping, error)
	GetByGuestMessage(botID uuid.UUID, guestChatID int64, guestMessageID int64) (*models.MessageMapping, error)
	GetByRecipientMessage(botID uuid.UUID, recipientChatID int64, recipientMessageID int64) (*models.MessageMapping, error)
	CountByBotIDAndDirection(botID uuid.UUID, direction models.MessageDirection) (int64, error)
}

type messageMappingRepository struct {
	db *gorm.DB
}

func NewMessageMappingRepository(db *gorm.DB) MessageMappingRepository {
	return &messageMappingRepository{db: db}
}

func (r *messageMappingRepository) Create(mapping *models.MessageMapping) error {
	return r.db.Create(mapping).Error
}

func (r *messageMappingRepository) GetByID(id uuid.UUID) (*models.MessageMapping, error) {
	var mapping models.MessageMapping
	if err := r.db.First(&mapping, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &mapping, nil
}

func (r *messageMappingRepository) GetByGuestMessage(botID uuid.UUID, guestChatID int64, guestMessageID int64) (*models.MessageMapping, error) {
	var mapping models.MessageMapping
	if err := r.db.Where("bot_id = ? AND guest_chat_id = ? AND guest_message_id = ?",
		botID, guestChatID, guestMessageID).First(&mapping).Error; err != nil {
		return nil, err
	}
	return &mapping, nil
}

func (r *messageMappingRepository) GetByRecipientMessage(botID uuid.UUID, recipientChatID int64, recipientMessageID int64) (*models.MessageMapping, error) {
	var mapping models.MessageMapping
	if err := r.db.Where("bot_id = ? AND recipient_chat_id = ? AND recipient_message_id = ?",
		botID, recipientChatID, recipientMessageID).First(&mapping).Error; err != nil {
		return nil, err
	}
	return &mapping, nil
}

func (r *messageMappingRepository) CountByBotIDAndDirection(botID uuid.UUID, direction models.MessageDirection) (int64, error) {
	var count int64
	if err := r.db.Model(&models.MessageMapping{}).
		Where("bot_id = ? AND direction = ?", botID, direction).
		Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}
