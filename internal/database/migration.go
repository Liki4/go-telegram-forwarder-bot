package database

import (
	"fmt"
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
		&models.BlacklistApprovalMessage{},
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
	migrator := db.Migrator()
	dbType := db.Dialector.Name()

	// Helper function to create index with database-specific SQL
	createIndexSQL := func(table, name string, columns []string, unique bool) error {
		if migrator.HasIndex(table, name) {
			return nil
		}

		indexType := "INDEX"
		if unique {
			indexType = "UNIQUE INDEX"
		}

		columnsStr := ""
		for i, col := range columns {
			if i > 0 {
				columnsStr += ", "
			}
			columnsStr += col
		}

		var sql string
		if dbType == "mysql" {
			// MySQL doesn't support IF NOT EXISTS for CREATE INDEX
			sql = fmt.Sprintf("CREATE %s %s ON %s(%s)", indexType, name, table, columnsStr)
		} else {
			// PostgreSQL and SQLite support IF NOT EXISTS
			sql = fmt.Sprintf("CREATE %s IF NOT EXISTS %s ON %s(%s)", indexType, name, table, columnsStr)
		}

		if err := db.Exec(sql).Error; err != nil {
			// Ignore error if index already exists (for MySQL)
			if dbType == "mysql" {
				// Check if error is about duplicate key/index
				if err.Error() != "" {
					// Try to check if index exists again (might have been created concurrently)
					if migrator.HasIndex(table, name) {
						return nil
					}
				}
			}
			return fmt.Errorf("failed to create index %s: %w", name, err)
		}
		return nil
	}

	// MessageMapping composite indexes
	indexes := []struct {
		name    string
		table   string
		columns []string
		unique  bool
	}{
		{"idx_guest_message", "message_mappings", []string{"guest_chat_id", "guest_message_id"}, false},
		{"idx_recipient_message", "message_mappings", []string{"recipient_chat_id", "recipient_message_id"}, false},
		{"idx_bot_created", "message_mappings", []string{"bot_id", "created_at"}, false},
		{"idx_guest_bot_user", "guests", []string{"bot_id", "guest_user_id"}, true},
	}

	for _, idx := range indexes {
		if err := createIndexSQL(idx.table, idx.name, idx.columns, idx.unique); err != nil {
			return err
		}
	}

	// For MySQL, we cannot use partial indexes with WHERE clause
	// Instead, we rely on application-level uniqueness checks in BeforeCreate hooks
	// For PostgreSQL and SQLite, we can create partial unique indexes
	if dbType == "postgres" || dbType == "sqlite" {
		partialIndexes := []struct {
			name    string
			table   string
			columns []string
			where   string
		}{
			{"idx_recipient_bot_chat", "recipients", []string{"bot_id", "chat_id"}, "deleted_at IS NULL"},
			{"idx_bot_admin", "bot_admins", []string{"bot_id", "admin_user_id"}, "deleted_at IS NULL"},
		}

		for _, idx := range partialIndexes {
			if migrator.HasIndex(idx.table, idx.name) {
				continue
			}

			columnsStr := fmt.Sprintf("%s, %s", idx.columns[0], idx.columns[1])
			sql := fmt.Sprintf(
				"CREATE UNIQUE INDEX IF NOT EXISTS %s ON %s(%s) WHERE %s",
				idx.name,
				idx.table,
				columnsStr,
				idx.where,
			)
			if err := db.Exec(sql).Error; err != nil {
				return fmt.Errorf("failed to create partial unique index %s: %w", idx.name, err)
			}
		}
	}

	return nil
}
