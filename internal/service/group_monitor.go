package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go-telegram-forwarder-bot/internal/models"
	"go-telegram-forwarder-bot/internal/repository"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type GroupMonitor struct {
	botRepo       repository.BotRepository
	recipientRepo repository.RecipientRepository
	auditLogRepo  repository.AuditLogRepository
	logger        *zap.Logger
}

func NewGroupMonitor(
	botRepo repository.BotRepository,
	recipientRepo repository.RecipientRepository,
	auditLogRepo repository.AuditLogRepository,
	logger *zap.Logger,
) *GroupMonitor {
	return &GroupMonitor{
		botRepo:       botRepo,
		recipientRepo: recipientRepo,
		auditLogRepo:  auditLogRepo,
		logger:        logger,
	}
}

func (gm *GroupMonitor) CheckRecipient(ctx context.Context, bot *gotgbot.Bot, botID uuid.UUID, recipient *models.Recipient) bool {
	if recipient.RecipientType != models.RecipientTypeGroup {
		return true
	}

	// Try to get chat information
	chat, err := bot.GetChat(recipient.ChatID, nil)
	if err != nil {
		// Check if it's a 400/403 error (chat not found or bot blocked)
		errStr := err.Error()
		if strings.Contains(errStr, "400") || strings.Contains(errStr, "403") ||
			strings.Contains(errStr, "chat not found") || strings.Contains(errStr, "bot was blocked") {
			gm.logger.Info("Recipient chat is invalid, removing",
				zap.String("bot_id", botID.String()),
				zap.Int64("chat_id", recipient.ChatID),
				zap.Error(err))

			// Delete recipient
			if delErr := gm.recipientRepo.Delete(recipient.ID); delErr != nil {
				gm.logger.Error("Failed to delete invalid recipient",
					zap.String("bot_id", botID.String()),
					zap.Int64("chat_id", recipient.ChatID),
					zap.Error(delErr))
				return false
			}

			// Log audit
			details := fmt.Sprintf(`{"chat_id": %d, "reason": "chat_not_found_or_bot_blocked"}`, recipient.ChatID)
			auditLog := &models.AuditLog{
				ActionType:   models.AuditLogActionDelRecipient,
				ResourceType: "recipient",
				ResourceID:   recipient.ID,
				Details:      details,
			}
			gm.auditLogRepo.Create(auditLog)

			return false
		}
		return true
	}

	// Chat exists and is accessible
	_ = chat
	return true
}

func (gm *GroupMonitor) StartPeriodicCheck(ctx context.Context, bot *gotgbot.Bot, botID uuid.UUID) {
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	// Initial check
	gm.checkAllRecipients(ctx, bot, botID)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			gm.checkAllRecipients(ctx, bot, botID)
		}
	}
}

func (gm *GroupMonitor) checkAllRecipients(ctx context.Context, bot *gotgbot.Bot, botID uuid.UUID) {
	recipients, err := gm.recipientRepo.GetByBotID(botID)
	if err != nil {
		gm.logger.Warn("Failed to get recipients for periodic check",
			zap.String("bot_id", botID.String()),
			zap.Error(err))
		return
	}

	for _, recipient := range recipients {
		if !gm.CheckRecipient(ctx, bot, botID, recipient) {
			// Recipient was removed
			continue
		}
	}
}
