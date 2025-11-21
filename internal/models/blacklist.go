package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type BlacklistStatus string

const (
	BlacklistStatusPending  BlacklistStatus = "pending"
	BlacklistStatusApproved BlacklistStatus = "approved"
	BlacklistStatusRejected BlacklistStatus = "rejected"
)

type BlacklistRequestType string

const (
	BlacklistRequestTypeBan   BlacklistRequestType = "ban"
	BlacklistRequestTypeUnban BlacklistRequestType = "unban"
)

type Blacklist struct {
	ID            uuid.UUID            `gorm:"type:char(36);primary_key"`
	BotID         uuid.UUID            `gorm:"type:char(36);not null;index"`
	Bot           ForwarderBot         `gorm:"foreignKey:BotID"`
	GuestID       uuid.UUID            `gorm:"type:char(36);not null;index"`
	Guest         Guest                `gorm:"foreignKey:GuestID"`
	Status        BlacklistStatus      `gorm:"type:varchar(20);not null;default:'pending'"`
	RequestUserID uuid.UUID            `gorm:"type:char(36);not null"`
	RequestUser   User                 `gorm:"foreignKey:RequestUserID"`
	RequestType   BlacklistRequestType `gorm:"type:varchar(20);not null"`
	ApprovedAt    *time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
	DeletedAt     gorm.DeletedAt `gorm:"index"`
}

func (b *Blacklist) BeforeCreate(tx *gorm.DB) error {
	if b.ID == uuid.Nil {
		b.ID = uuid.New()
	}
	return nil
}
