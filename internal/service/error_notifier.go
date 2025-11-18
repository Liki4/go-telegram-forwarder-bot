package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go-telegram-forwarder-bot/internal/config"
	"go-telegram-forwarder-bot/internal/utils"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"go.uber.org/zap"
)

type ErrorNotifier struct {
	bot          *gotgbot.Bot
	superusers   []int64
	logger       *zap.Logger
	notifiedErrs map[string]time.Time
	mutex        sync.RWMutex
}

type ErrorType string

const (
	ErrorTypeDatabase ErrorType = "database"
	ErrorTypeRedis    ErrorType = "redis"
	ErrorTypeBotToken ErrorType = "bot_token"
	ErrorTypeSystem   ErrorType = "system"
)

func NewErrorNotifier(bot *gotgbot.Bot, cfg *config.Config, logger *zap.Logger) *ErrorNotifier {
	return &ErrorNotifier{
		bot:          bot,
		superusers:   cfg.ManagerBot.Superusers,
		logger:       logger,
		notifiedErrs: make(map[string]time.Time),
	}
}

func (en *ErrorNotifier) NotifyCriticalError(ctx context.Context, errType ErrorType, err error, details string) {
	en.mutex.Lock()
	defer en.mutex.Unlock()

	key := string(errType)
	lastNotified, exists := en.notifiedErrs[key]

	// Check if we should notify (1 hour debounce)
	if exists && time.Since(lastNotified) < 1*time.Hour {
		en.logger.Debug("Error notification skipped due to debounce",
			zap.String("error_type", key),
			zap.Time("last_notified", lastNotified))
		return
	}

	// Update last notified time
	en.notifiedErrs[key] = time.Now()

	// Format error message
	message := fmt.Sprintf(
		"*Critical Error Alert*\n\n"+
			"Type: `%s`\n"+
			"Error: `%s`\n"+
			"Details: `%s`\n"+
			"Time: %s",
		string(errType),
		utils.EscapeMarkdown(fmt.Sprintf("%v", err)),
		utils.EscapeMarkdown(details),
		time.Now().Format("2006-01-02 15:04:05"),
	)

	// Notify all superusers
	for _, superuserID := range en.superusers {
		_, sendErr := en.bot.SendMessage(superuserID, message, &gotgbot.SendMessageOpts{
			ParseMode: "Markdown",
		})
		if sendErr != nil {
			en.logger.Warn("Failed to send error notification to superuser",
				zap.Int64("superuser_id", superuserID),
				zap.Error(sendErr))
		}
	}

	en.logger.Error("Critical error notified to superusers",
		zap.String("error_type", key),
		zap.Error(err))
}
