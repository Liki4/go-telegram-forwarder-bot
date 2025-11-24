package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// BlacklistApprovalMessage stores the message ID sent to each manager/admin for approval requests
type BlacklistApprovalMessage struct {
	ID          uuid.UUID `gorm:"type:char(36);primary_key"`
	BlacklistID uuid.UUID `gorm:"type:char(36);not null;index"`
	Blacklist   Blacklist `gorm:"foreignKey:BlacklistID"`
	UserID      uuid.UUID `gorm:"type:char(36);not null;index"`
	User        User      `gorm:"foreignKey:UserID"`
	ChatID      int64     `gorm:"not null"`
	MessageID   int64     `gorm:"not null"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
	DeletedAt   gorm.DeletedAt `gorm:"index"`
}

func (b *BlacklistApprovalMessage) BeforeCreate(tx *gorm.DB) error {
	if b.ID == uuid.Nil {
		b.ID = uuid.New()
	}
	return nil
}
