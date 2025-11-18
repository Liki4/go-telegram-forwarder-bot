package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Guest struct {
	ID          uuid.UUID    `gorm:"type:uuid;primary_key"`
	BotID       uuid.UUID    `gorm:"type:uuid;not null;index"`
	Bot         ForwarderBot `gorm:"foreignKey:BotID"`
	GuestUserID int64        `gorm:"not null"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (g *Guest) BeforeCreate(tx *gorm.DB) error {
	if g.ID == uuid.Nil {
		g.ID = uuid.New()
	}
	return nil
}
