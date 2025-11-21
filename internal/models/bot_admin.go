package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type BotAdmin struct {
	ID          uuid.UUID    `gorm:"type:char(36);primary_key"`
	BotID       uuid.UUID    `gorm:"type:char(36);not null;index"`
	Bot         ForwarderBot `gorm:"foreignKey:BotID"`
	AdminUserID uuid.UUID    `gorm:"type:char(36);not null;index"`
	AdminUser   User         `gorm:"foreignKey:AdminUserID"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
	DeletedAt   gorm.DeletedAt `gorm:"index"`
}

func (ba *BotAdmin) BeforeCreate(tx *gorm.DB) error {
	if ba.ID == uuid.Nil {
		ba.ID = uuid.New()
	}

	// Check uniqueness
	var count int64
	tx.Model(&BotAdmin{}).
		Where("bot_id = ? AND admin_user_id = ? AND deleted_at IS NULL", ba.BotID, ba.AdminUserID).
		Count(&count)
	if count > 0 {
		return gorm.ErrDuplicatedKey
	}
	return nil
}
