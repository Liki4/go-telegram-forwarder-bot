package message

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"go-telegram-forwarder-bot/internal/config"
	"go-telegram-forwarder-bot/internal/models"
	"go-telegram-forwarder-bot/internal/repository"
	"go-telegram-forwarder-bot/internal/service"
	"go-telegram-forwarder-bot/internal/utils"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type Forwarder struct {
	botRepo            repository.BotRepository
	recipientRepo      repository.RecipientRepository
	guestRepo          repository.GuestRepository
	messageMappingRepo repository.MessageMappingRepository
	rateLimiter        *RateLimiter
	retryHandler       *RetryHandler
	config             *config.Config
	logger             *zap.Logger
	groupMonitor       GroupMonitorInterface
	errorNotifier      ErrorNotifierInterface
	managerNotifier    ManagerNotifierInterface
}

type ManagerNotifierInterface interface {
	NotifyManager(ctx context.Context, botID uuid.UUID, message string) error
}

type ErrorNotifierInterface interface {
	NotifyCriticalError(ctx context.Context, errType service.ErrorType, err error, details string)
}

type GroupMonitorInterface interface {
	CheckRecipient(ctx context.Context, bot *gotgbot.Bot, botID uuid.UUID, recipient *models.Recipient) bool
}

type ForwardResult struct {
	SuccessCount int
	FailureCount int
	Errors       []error
}

func NewForwarder(
	botRepo repository.BotRepository,
	recipientRepo repository.RecipientRepository,
	guestRepo repository.GuestRepository,
	messageMappingRepo repository.MessageMappingRepository,
	rateLimiter *RateLimiter,
	retryHandler *RetryHandler,
	cfg *config.Config,
	logger *zap.Logger,
) *Forwarder {
	return &Forwarder{
		botRepo:            botRepo,
		recipientRepo:      recipientRepo,
		guestRepo:          guestRepo,
		messageMappingRepo: messageMappingRepo,
		rateLimiter:        rateLimiter,
		retryHandler:       retryHandler,
		config:             cfg,
		logger:             logger,
	}
}

func (f *Forwarder) SetGroupMonitor(monitor GroupMonitorInterface) {
	f.groupMonitor = monitor
}

func (f *Forwarder) SetErrorNotifier(notifier ErrorNotifierInterface) {
	f.errorNotifier = notifier
}

func (f *Forwarder) SetManagerNotifier(notifier ManagerNotifierInterface) {
	f.managerNotifier = notifier
}

func (f *Forwarder) ForwardToRecipients(
	ctx context.Context,
	bot *gotgbot.Bot,
	botID uuid.UUID,
	guestChatID int64,
	message *gotgbot.Message,
) (*ForwardResult, error) {
	messageID := message.MessageId

	f.logger.Debug("Starting message forwarding",
		zap.String("bot_id", botID.String()),
		zap.Int64("message_id", messageID),
		zap.Int64("guest_chat_id", guestChatID))

	f.logger.Debug("Retrieving recipients for bot",
		zap.String("bot_id", botID.String()))
	recipients, err := f.recipientRepo.GetByBotID(botID)
	if err != nil {
		f.logger.Debug("Failed to get recipients",
			zap.String("bot_id", botID.String()),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get recipients: %w", err)
	}

	f.logger.Debug("Recipients retrieved",
		zap.String("bot_id", botID.String()),
		zap.Int("recipient_count", len(recipients)))

	if len(recipients) == 0 {
		f.logger.Debug("No recipients found, skipping forwarding",
			zap.String("bot_id", botID.String()),
			zap.Int64("message_id", messageID))
		return &ForwardResult{SuccessCount: 0, FailureCount: 0}, nil
	}

	f.logger.Debug("Getting or creating guest record",
		zap.String("bot_id", botID.String()),
		zap.Int64("guest_chat_id", guestChatID))
	_, err = f.guestRepo.GetOrCreateByBotIDAndUserID(botID, guestChatID)
	if err != nil {
		f.logger.Debug("Failed to get or create guest",
			zap.String("bot_id", botID.String()),
			zap.Int64("guest_chat_id", guestChatID),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get or create guest: %w", err)
	}
	f.logger.Debug("Guest record retrieved/created",
		zap.String("bot_id", botID.String()),
		zap.Int64("guest_chat_id", guestChatID))

	// Check guest message rate limit
	// If rate limit exceeded, delay sending by waiting
	f.logger.Debug("Checking guest message rate limit",
		zap.String("bot_id", botID.String()),
		zap.Int64("guest_chat_id", guestChatID))
	if !f.rateLimiter.AllowGuestMessage(ctx, botID, guestChatID) {
		f.logger.Warn("Guest message rate limit exceeded, delaying send",
			zap.String("bot_id", botID.String()),
			zap.Int64("guest_chat_id", guestChatID))
		// Delay sending: wait for 1 second (rate limit window)
		f.logger.Debug("Waiting 1 second for rate limit window",
			zap.String("bot_id", botID.String()),
			zap.Int64("guest_chat_id", guestChatID))
		select {
		case <-ctx.Done():
			f.logger.Debug("Context cancelled during rate limit delay",
				zap.String("bot_id", botID.String()),
				zap.Int64("guest_chat_id", guestChatID))
			return nil, ctx.Err()
		case <-time.After(1 * time.Second):
			// Retry rate limit check after delay
			f.logger.Debug("Rechecking rate limit after delay",
				zap.String("bot_id", botID.String()),
				zap.Int64("guest_chat_id", guestChatID))
			if !f.rateLimiter.AllowGuestMessage(ctx, botID, guestChatID) {
				f.logger.Warn("Guest message still rate limited after delay",
					zap.String("bot_id", botID.String()),
					zap.Int64("guest_chat_id", guestChatID))
				// Continue anyway to avoid blocking indefinitely
			} else {
				f.logger.Debug("Rate limit cleared after delay",
					zap.String("bot_id", botID.String()),
					zap.Int64("guest_chat_id", guestChatID))
			}
		}
	} else {
		f.logger.Debug("Guest message rate limit check passed",
			zap.String("bot_id", botID.String()),
			zap.Int64("guest_chat_id", guestChatID))
	}

	f.logger.Debug("Starting concurrent forwarding to recipients",
		zap.String("bot_id", botID.String()),
		zap.Int64("message_id", messageID),
		zap.Int("recipient_count", len(recipients)))

	var wg sync.WaitGroup
	var mu sync.Mutex
	result := &ForwardResult{
		Errors: make([]error, 0),
	}

	for i, recipient := range recipients {
		wg.Add(1)
		go func(rec *models.Recipient, index int) {
			defer wg.Done()

			f.logger.Debug("Starting forwarding to recipient",
				zap.String("bot_id", botID.String()),
				zap.Int64("message_id", messageID),
				zap.Int64("recipient_chat_id", rec.ChatID),
				zap.String("recipient_type", string(rec.RecipientType)),
				zap.Int("recipient_index", index))

			f.logger.Debug("Checking Telegram API rate limit",
				zap.String("bot_id", botID.String()),
				zap.Int64("recipient_chat_id", rec.ChatID))
			if !f.rateLimiter.AllowTelegramAPI(ctx) {
				f.logger.Warn("Rate limit exceeded for Telegram API",
					zap.String("bot_id", botID.String()),
					zap.Int64("recipient_chat_id", rec.ChatID))
				mu.Lock()
				result.FailureCount++
				result.Errors = append(result.Errors, fmt.Errorf("rate limit exceeded"))
				mu.Unlock()
				f.logger.Debug("Skipping forwarding due to rate limit",
					zap.String("bot_id", botID.String()),
					zap.Int64("recipient_chat_id", rec.ChatID))
				return
			}

			f.logger.Debug("Rate limit check passed, starting retry handler",
				zap.String("bot_id", botID.String()),
				zap.Int64("recipient_chat_id", rec.ChatID),
				zap.Int("max_attempts", f.config.Retry.MaxAttempts))
			err := f.retryHandler.Retry(ctx, func() error {
				f.logger.Debug("Attempting to forward message",
					zap.String("bot_id", botID.String()),
					zap.Int64("message_id", messageID),
					zap.Int64("guest_chat_id", guestChatID),
					zap.Int64("recipient_chat_id", rec.ChatID))
				return f.forwardMessage(ctx, bot, botID, guestChatID, message.MessageId, rec.ChatID, rec)
			})

			mu.Lock()
			if err != nil {
				result.FailureCount++
				result.Errors = append(result.Errors, err)
				f.logger.Warn("Failed to forward message after retries",
					zap.String("bot_id", botID.String()),
					zap.Int64("message_id", messageID),
					zap.Int64("recipient_chat_id", rec.ChatID),
					zap.Int("max_attempts", f.config.Retry.MaxAttempts),
					zap.Error(err))

				// Send failure notification to recipient
				f.logger.Debug("Sending failure notification to recipient",
					zap.String("bot_id", botID.String()),
					zap.Int64("recipient_chat_id", rec.ChatID))
				f.sendFailureNotification(ctx, bot, rec.ChatID, err, f.config.Retry.MaxAttempts)

				// Check if it's a 401 error (Bot Token invalid)
				errStr := err.Error()
				if strings.Contains(errStr, "401") || strings.Contains(errStr, "Unauthorized") {
					f.logger.Debug("Detected 401 error, notifying critical error",
						zap.String("bot_id", botID.String()),
						zap.Int64("recipient_chat_id", rec.ChatID))
					if f.errorNotifier != nil {
						f.errorNotifier.NotifyCriticalError(ctx, service.ErrorTypeBotToken, err,
							fmt.Sprintf("Bot ID: %s, Chat ID: %d", botID.String(), rec.ChatID))
					}
				}

				// Check if recipient is invalid (group deleted or bot blocked)
				if f.groupMonitor != nil {
					f.logger.Debug("Checking recipient validity",
						zap.String("bot_id", botID.String()),
						zap.Int64("recipient_chat_id", rec.ChatID))
					if !f.groupMonitor.CheckRecipient(ctx, bot, botID, rec) {
						f.logger.Info("Invalid recipient detected and removed",
							zap.String("bot_id", botID.String()),
							zap.Int64("recipient_chat_id", rec.ChatID))
					}
				}
			} else {
				result.SuccessCount++
				f.logger.Debug("Message forwarded successfully",
					zap.String("bot_id", botID.String()),
					zap.Int64("message_id", messageID),
					zap.Int64("recipient_chat_id", rec.ChatID))
			}
			mu.Unlock()
		}(recipient, i)
	}

	f.logger.Debug("Waiting for all forwarding goroutines to complete",
		zap.String("bot_id", botID.String()),
		zap.Int64("message_id", messageID),
		zap.Int("recipient_count", len(recipients)))
	wg.Wait()
	f.logger.Debug("All forwarding goroutines completed",
		zap.String("bot_id", botID.String()),
		zap.Int64("message_id", messageID),
		zap.Int("success_count", result.SuccessCount),
		zap.Int("failure_count", result.FailureCount))

	// If there are failures after all retries, notify Manager
	// According to requirements: "重试到最后失败则无需执行任何动作，通知 Manager 发生失败了"
	if result.FailureCount > 0 && f.managerNotifier != nil {
		f.logger.Debug("Preparing manager notification for batch forwarding failure",
			zap.String("bot_id", botID.String()),
			zap.Int64("message_id", messageID),
			zap.Int("failure_count", result.FailureCount))
		errorSummary := make([]string, 0, len(result.Errors))
		for _, err := range result.Errors {
			errorSummary = append(errorSummary, utils.EscapeMarkdown(err.Error()))
		}
		notificationMsg := fmt.Sprintf(
			"*Batch Forwarding Failed*\n\n"+
				"Bot ID: `%s`\n"+
				"Success: %d\n"+
				"Failures: %d\n"+
				"Retry Attempts: %d\n"+
				"Errors:\n%s\n"+
				"Time: %s",
			botID.String(),
			result.SuccessCount,
			result.FailureCount,
			f.config.Retry.MaxAttempts,
			strings.Join(errorSummary, "\n"),
			time.Now().Format("2006-01-02 15:04:05"),
		)
		if notifyErr := f.managerNotifier.NotifyManager(ctx, botID, notificationMsg); notifyErr != nil {
			f.logger.Warn("Failed to notify manager about batch forwarding failure",
				zap.String("bot_id", botID.String()),
				zap.Error(notifyErr))
		} else {
			f.logger.Debug("Manager notification sent successfully",
				zap.String("bot_id", botID.String()),
				zap.Int64("message_id", messageID))
		}
	}

	f.logger.Debug("Message forwarding completed",
		zap.String("bot_id", botID.String()),
		zap.Int64("message_id", messageID),
		zap.Int("success_count", result.SuccessCount),
		zap.Int("failure_count", result.FailureCount))
	return result, nil
}

func (f *Forwarder) forwardMessage(
	_ context.Context,
	bot *gotgbot.Bot,
	botID uuid.UUID,
	guestChatID int64,
	guestMessageID int64,
	recipientChatID int64,
	_ *models.Recipient,
) error {
	f.logger.Debug("Calling Telegram API to forward message",
		zap.String("bot_id", botID.String()),
		zap.Int64("guest_chat_id", guestChatID),
		zap.Int64("guest_message_id", guestMessageID),
		zap.Int64("recipient_chat_id", recipientChatID))
	forwardedMsg, err := bot.ForwardMessage(recipientChatID, guestChatID, guestMessageID, nil)
	if err != nil {
		f.logger.Debug("Telegram API forward message failed",
			zap.String("bot_id", botID.String()),
			zap.Int64("guest_message_id", guestMessageID),
			zap.Int64("recipient_chat_id", recipientChatID),
			zap.Error(err))
		return fmt.Errorf("failed to forward message: %w", err)
	}

	f.logger.Debug("Message forwarded successfully via Telegram API",
		zap.String("bot_id", botID.String()),
		zap.Int64("guest_message_id", guestMessageID),
		zap.Int64("recipient_chat_id", recipientChatID),
		zap.Int64("forwarded_message_id", forwardedMsg.MessageId))

	mapping := &models.MessageMapping{
		BotID:              botID,
		GuestChatID:        guestChatID,
		GuestMessageID:     guestMessageID,
		RecipientChatID:    recipientChatID,
		RecipientMessageID: forwardedMsg.MessageId,
		Direction:          models.MessageDirectionInbound,
	}

	f.logger.Debug("Creating message mapping record",
		zap.String("bot_id", botID.String()),
		zap.Int64("guest_message_id", guestMessageID),
		zap.Int64("recipient_message_id", forwardedMsg.MessageId))
	if err := f.messageMappingRepo.Create(mapping); err != nil {
		f.logger.Warn("Failed to create message mapping",
			zap.String("bot_id", botID.String()),
			zap.Int64("guest_message_id", guestMessageID),
			zap.Int64("recipient_message_id", forwardedMsg.MessageId),
			zap.Error(err))
	} else {
		f.logger.Debug("Message mapping created successfully",
			zap.String("bot_id", botID.String()),
			zap.Int64("guest_message_id", guestMessageID),
			zap.Int64("recipient_message_id", forwardedMsg.MessageId))
	}

	return nil
}

func (f *Forwarder) sendFailureNotification(
	_ context.Context,
	bot *gotgbot.Bot,
	recipientChatID int64,
	err error,
	retryAttempts int,
) {
	message := fmt.Sprintf(
		"*Message Forwarding Failed*\n\n"+
			"Error: `%s`\n"+
			"Retry Attempts: %d\n"+
			"Time: %s",
		utils.EscapeMarkdown(fmt.Sprintf("%v", err)), retryAttempts, time.Now().Format("2006-01-02 15:04:05"),
	)

	_, sendErr := bot.SendMessage(recipientChatID, message, &gotgbot.SendMessageOpts{
		ParseMode: "Markdown",
	})
	if sendErr != nil {
		f.logger.Warn("Failed to send failure notification",
			zap.Int64("recipient_chat_id", recipientChatID),
			zap.Error(sendErr))
	}
}

func (f *Forwarder) ForwardReplyToGuest(
	ctx context.Context,
	bot *gotgbot.Bot,
	botID uuid.UUID,
	recipientChatID int64,
	replyMessage *gotgbot.Message,
) error {
	if replyMessage.ReplyToMessage == nil {
		return fmt.Errorf("message is not a reply")
	}

	recipientMessageID := replyMessage.ReplyToMessage.MessageId

	mapping, err := f.messageMappingRepo.GetByRecipientMessage(
		botID, recipientChatID, recipientMessageID)
	if err != nil {
		return fmt.Errorf("failed to find message mapping: %w", err)
	}

	if !f.rateLimiter.AllowTelegramAPI(ctx) {
		return fmt.Errorf("rate limit exceeded")
	}

	return f.retryHandler.Retry(ctx, func() error {
		_, err := bot.ForwardMessage(
			mapping.GuestChatID,
			recipientChatID,
			replyMessage.MessageId,
			nil)
		if err != nil {
			return fmt.Errorf("failed to forward reply: %w", err)
		}

		replyMapping := &models.MessageMapping{
			BotID:              botID,
			GuestChatID:        mapping.GuestChatID,
			GuestMessageID:     mapping.GuestMessageID,
			RecipientChatID:    recipientChatID,
			RecipientMessageID: replyMessage.MessageId,
			Direction:          models.MessageDirectionOutbound,
		}

		if err := f.messageMappingRepo.Create(replyMapping); err != nil {
			f.logger.Warn("Failed to create reply mapping",
				zap.String("bot_id", botID.String()),
				zap.Error(err))
		}

		return nil
	})
}
