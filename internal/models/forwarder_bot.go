package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type ForwarderBot struct {
	ID        uuid.UUID `gorm:"type:char(36);primary_key"`
	Token     string    `gorm:"type:varchar(500);not null"`
	Name      string    `gorm:"type:varchar(255)"`
	ManagerID uuid.UUID `gorm:"type:char(36);not null;index"`
	Manager   User      `gorm:"foreignKey:ManagerID"`
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"index"`
}

func (b *ForwarderBot) BeforeCreate(tx *gorm.DB) error {
	if b.ID == uuid.Nil {
		b.ID = uuid.New()
	}
	return nil
}
