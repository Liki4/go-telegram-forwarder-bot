package service

import (
	"context"
	"fmt"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/google/uuid"
	"go-telegram-forwarder-bot/internal/repository"
	"go.uber.org/zap"
)

type ManagerNotifier struct {
	managerBot *gotgbot.Bot
	botRepo    repository.BotRepository
	userRepo   repository.UserRepository
	logger     *zap.Logger
}

func NewManagerNotifier(
	managerBot *gotgbot.Bot,
	botRepo repository.BotRepository,
	userRepo repository.UserRepository,
	logger *zap.Logger,
) *ManagerNotifier {
	return &ManagerNotifier{
		managerBot: managerBot,
		botRepo:    botRepo,
		userRepo:   userRepo,
		logger:     logger,
	}
}

func (mn *ManagerNotifier) NotifyManager(ctx context.Context, botID uuid.UUID, message string) error {
	// Get bot to find manager
	bot, err := mn.botRepo.GetByID(botID)
	if err != nil {
		return fmt.Errorf("failed to get bot: %w", err)
	}

	// Get manager user
	manager, err := mn.userRepo.GetByID(bot.ManagerID)
	if err != nil {
		return fmt.Errorf("failed to get manager: %w", err)
	}

	// Send notification via ManagerBot
	_, sendErr := mn.managerBot.SendMessage(manager.TelegramUserID, message, &gotgbot.SendMessageOpts{
		ParseMode: "Markdown",
	})
	if sendErr != nil {
		mn.logger.Warn("Failed to send manager notification",
			zap.String("bot_id", botID.String()),
			zap.Int64("manager_telegram_id", manager.TelegramUserID),
			zap.Error(sendErr))
		return fmt.Errorf("failed to send notification: %w", sendErr)
	}

	mn.logger.Info("Manager notified about batch forwarding failure",
		zap.String("bot_id", botID.String()),
		zap.Int64("manager_telegram_id", manager.TelegramUserID))

	return nil
}
