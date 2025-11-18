package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type AuditLogAction string

const (
	AuditLogActionAddBot       AuditLogAction = "add_bot"
	AuditLogActionDeleteBot    AuditLogAction = "delete_bot"
	AuditLogActionBan          AuditLogAction = "ban"
	AuditLogActionUnban        AuditLogAction = "unban"
	AuditLogActionAddAdmin     AuditLogAction = "add_admin"
	AuditLogActionDelAdmin     AuditLogAction = "del_admin"
	AuditLogActionAddRecipient AuditLogAction = "add_recipient"
	AuditLogActionDelRecipient AuditLogAction = "del_recipient"
)

type AuditLog struct {
	ID           uuid.UUID      `gorm:"type:uuid;primary_key"`
	UserID       *uuid.UUID     `gorm:"type:uuid;index"`
	User         *User          `gorm:"foreignKey:UserID"`
	ActionType   AuditLogAction `gorm:"type:varchar(50);not null;index"`
	ResourceType string         `gorm:"type:varchar(50);not null"`
	ResourceID   uuid.UUID      `gorm:"type:uuid;not null"`
	Details      string         `gorm:"type:text"`
	CreatedAt    time.Time      `gorm:"index"`
}

func (a *AuditLog) BeforeCreate(tx *gorm.DB) error {
	if a.ID == uuid.Nil {
		a.ID = uuid.New()
	}
	return nil
}
