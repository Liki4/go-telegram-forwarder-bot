package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type MessageDirection string

const (
	MessageDirectionInbound  MessageDirection = "inbound"
	MessageDirectionOutbound MessageDirection = "outbound"
)

type MessageMapping struct {
	ID                 uuid.UUID        `gorm:"type:char(36);primary_key"`
	BotID              uuid.UUID        `gorm:"type:char(36);not null;index:idx_bot_created"`
	Bot                ForwarderBot     `gorm:"foreignKey:BotID"`
	GuestChatID        int64            `gorm:"not null;index:idx_guest_message"`
	GuestMessageID     int64            `gorm:"not null;index:idx_guest_message"`
	RecipientChatID    int64            `gorm:"not null;index:idx_recipient_message"`
	RecipientMessageID int64            `gorm:"not null;index:idx_recipient_message"`
	Direction          MessageDirection `gorm:"type:varchar(20);not null"`
	CreatedAt          time.Time        `gorm:"index:idx_bot_created"`
}

func (m *MessageMapping) BeforeCreate(tx *gorm.DB) error {
	if m.ID == uuid.Nil {
		m.ID = uuid.New()
	}
	return nil
}
