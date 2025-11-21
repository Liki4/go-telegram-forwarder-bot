package models

import (
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type RecipientType string

const (
	RecipientTypeUser  RecipientType = "user"
	RecipientTypeGroup RecipientType = "group"
)

type Recipient struct {
	ID            uuid.UUID     `gorm:"type:char(36);primary_key"`
	BotID         uuid.UUID     `gorm:"type:char(36);not null;index"`
	Bot           ForwarderBot  `gorm:"foreignKey:BotID"`
	RecipientType RecipientType `gorm:"type:varchar(20);not null"`
	ChatID        int64         `gorm:"not null"`
	CreatedAt     time.Time
	UpdatedAt     time.Time
	DeletedAt     gorm.DeletedAt `gorm:"index"`
}

func (r *Recipient) BeforeCreate(tx *gorm.DB) error {
	if r.ID == uuid.Nil {
		r.ID = uuid.New()
	}

	// Check uniqueness
	var count int64
	tx.Model(&Recipient{}).
		Where("bot_id = ? AND chat_id = ? AND deleted_at IS NULL", r.BotID, r.ChatID).
		Count(&count)
	if count > 0 {
		return errors.New("recipient already exists")
	}
	return nil
}
