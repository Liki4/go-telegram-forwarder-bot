package database

import (
	"go-telegram-forwarder-bot/internal/models"
	"gorm.io/gorm"
)

func Migrate(db *gorm.DB) error {
	if err := db.AutoMigrate(
		&models.User{},
		&models.ForwarderBot{},
		&models.BotAdmin{},
		&models.Recipient{},
		&models.Guest{},
		&models.Blacklist{},
		&models.MessageMapping{},
		&models.AuditLog{},
	); err != nil {
		return err
	}

	// Create composite indexes
	if err := createIndexes(db); err != nil {
		return err
	}

	return nil
}

func createIndexes(db *gorm.DB) error {
	// MessageMapping composite indexes
	if err := db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_guest_message 
		ON message_mappings(guest_chat_id, guest_message_id)
	`).Error; err != nil {
		return err
	}

	if err := db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_recipient_message 
		ON message_mappings(recipient_chat_id, recipient_message_id)
	`).Error; err != nil {
		return err
	}

	if err := db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_bot_created 
		ON message_mappings(bot_id, created_at)
	`).Error; err != nil {
		return err
	}

	// Guest unique index
	if err := db.Exec(`
		CREATE UNIQUE INDEX IF NOT EXISTS idx_guest_bot_user 
		ON guests(bot_id, guest_user_id)
	`).Error; err != nil {
		return err
	}

	// Recipient unique index
	if err := db.Exec(`
		CREATE UNIQUE INDEX IF NOT EXISTS idx_recipient_bot_chat 
		ON recipients(bot_id, chat_id) WHERE deleted_at IS NULL
	`).Error; err != nil {
		return err
	}

	// BotAdmin unique index
	if err := db.Exec(`
		CREATE UNIQUE INDEX IF NOT EXISTS idx_bot_admin 
		ON bot_admins(bot_id, admin_user_id) WHERE deleted_at IS NULL
	`).Error; err != nil {
		return err
	}

	return nil
}
