package manager_bot

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"go-telegram-forwarder-bot/internal/models"
	"go-telegram-forwarder-bot/internal/utils"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

func (s *Service) handleAddBot(ctx context.Context, b *gotgbot.Bot, update *ext.Context) error {
	userID := update.EffectiveUser.Id
	chatID := update.EffectiveChat.Id
	parts := strings.Fields(update.EffectiveMessage.Text)

	s.logger.Debug("Processing /addbot command",
		zap.Int64("user_id", userID),
		zap.Int64("chat_id", chatID),
		zap.Int("parts_count", len(parts)),
		zap.Strings("parts", parts))

	if len(parts) < 2 {
		s.logger.Debug("Invalid /addbot command format - missing token",
			zap.Int64("user_id", userID),
			zap.Int("parts_count", len(parts)))
		_, err := b.SendMessage(update.EffectiveChat.Id,
			"Usage: /addbot <token>\nExample: /addbot 123456789:ABCdefGHIjklMNOpqrsTUVwxyz", nil)
		return err
	}

	// Send "please wait" message first
	waitMsg, err := b.SendMessage(update.EffectiveChat.Id,
		"⏳ Processing, please wait...", nil)
	if err != nil {
		s.logger.Warn("Failed to send wait message", zap.Error(err))
		// Continue anyway, but we won't be able to update the message
	}
	var waitMessageID int64
	if waitMsg != nil {
		waitMessageID = waitMsg.MessageId
		s.logger.Debug("Wait message sent",
			zap.Int64("user_id", userID),
			zap.Int64("message_id", waitMessageID))
	}

	// Helper function to update wait message
	updateWaitMessage := func(text string) {
		if waitMessageID > 0 {
			_, _, editErr := b.EditMessageText(text, &gotgbot.EditMessageTextOpts{
				ChatId:    update.EffectiveChat.Id,
				MessageId: waitMessageID,
				ParseMode: "Markdown",
			})
			if editErr != nil {
				s.logger.Warn("Failed to update wait message",
					zap.Int64("user_id", userID),
					zap.Int64("message_id", waitMessageID),
					zap.Error(editErr))
			}
		}
	}

	token := parts[1]
	tokenPrefix := token
	if len(token) > 10 {
		tokenPrefix = token[:10] + "..."
	}
	s.logger.Debug("Extracted bot token",
		zap.Int64("user_id", userID),
		zap.String("token_prefix", tokenPrefix),
		zap.Int("token_length", len(token)))

	// Validate token by calling Telegram API
	// Use proxy if enabled
	s.logger.Debug("Validating bot token",
		zap.Int64("user_id", userID),
		zap.Bool("proxy_enabled", s.config.Proxy.Enabled))

	var botOpts *gotgbot.BotOpts
	if s.config.Proxy.Enabled {
		s.logger.Debug("Creating HTTP client with proxy",
			zap.Int64("user_id", userID),
			zap.String("proxy_url", s.config.Proxy.URL))
		httpClient, err := utils.CreateHTTPClientWithProxy(&s.config.Proxy)
		if err != nil {
			// If proxy is enabled but creation fails, return error immediately
			// Do not fallback to direct connection to avoid timeout issues
			s.logger.Error("Failed to create proxy HTTP client",
				zap.Int64("user_id", userID),
				zap.String("proxy_url", s.config.Proxy.URL),
				zap.Error(err))
			updateWaitMessage(fmt.Sprintf("❌ Proxy configuration error: `%s`", utils.EscapeMarkdown(err.Error())))
			return fmt.Errorf("failed to create proxy HTTP client: %w", err)
		}

		botClient := &gotgbot.BaseBotClient{
			Client:             *httpClient,
			UseTestEnvironment: false,
			DefaultRequestOpts: nil,
		}
		botOpts = &gotgbot.BotOpts{
			BotClient: botClient,
		}
		s.logger.Debug("Proxy HTTP client created successfully",
			zap.Int64("user_id", userID),
			zap.String("proxy_url", s.config.Proxy.URL))
	}

	s.logger.Debug("Creating bot instance for validation",
		zap.Int64("user_id", userID))
	testBot, err := gotgbot.NewBot(token, botOpts)
	if err != nil {
		s.logger.Debug("Failed to create bot instance for validation",
			zap.Int64("user_id", userID),
			zap.Error(err))
		updateWaitMessage(fmt.Sprintf("❌ Invalid bot token: `%v`", utils.EscapeMarkdown(fmt.Sprintf("%v", err))))
		return err
	}

	s.logger.Debug("Bot instance created, calling GetMe to verify token",
		zap.Int64("user_id", userID))
	botInfo, err := testBot.GetMe(nil)
	if err != nil {
		s.logger.Debug("Failed to verify bot token via GetMe",
			zap.Int64("user_id", userID),
			zap.Error(err))
		updateWaitMessage(fmt.Sprintf("❌ Failed to verify bot token: `%s`", utils.EscapeMarkdown(fmt.Sprintf("%v", err))))
		return err
	}

	s.logger.Debug("Bot token verified successfully",
		zap.Int64("user_id", userID),
		zap.String("bot_username", botInfo.Username),
		zap.Int64("bot_id", botInfo.Id),
		zap.Bool("bot_is_bot", botInfo.IsBot))

	// Get or create user
	username := update.EffectiveUser.Username
	var usernamePtr *string
	if username != "" {
		usernamePtr = &username
	}

	s.logger.Debug("Getting or creating user",
		zap.Int64("user_id", userID),
		zap.String("username", username))
	user, err := s.userRepo.GetOrCreateByTelegramUserID(
		update.EffectiveUser.Id,
		usernamePtr)
	if err != nil {
		s.logger.Error("Failed to get or create user", zap.Error(err))
		updateWaitMessage("❌ An error occurred. Please try again later.")
		return err
	}
	s.logger.Debug("User retrieved/created",
		zap.Int64("user_id", userID),
		zap.String("user_uuid", user.ID.String()))

	// Check if bot already exists by trying to encrypt and compare
	// Since tokens are encrypted, we need to check by bot username or ID
	// For now, we'll check after encryption by comparing all bots
	// This is not perfect but works for the use case
	s.logger.Debug("Checking if bot already exists",
		zap.Int64("user_id", userID),
		zap.String("bot_username", botInfo.Username))
	allBots, err := s.botRepo.GetAll()
	if err == nil {
		s.logger.Debug("Retrieved all bots for duplicate check",
			zap.Int64("user_id", userID),
			zap.Int("total_bots", len(allBots)))
		for _, existingBot := range allBots {
			decryptedToken, decryptErr := utils.DecryptToken(existingBot.Token, s.encryptionKey)
			if decryptErr == nil && decryptedToken == token {
				s.logger.Debug("Bot already exists",
					zap.Int64("user_id", userID),
					zap.String("bot_username", botInfo.Username),
					zap.String("existing_bot_id", existingBot.ID.String()))
				updateWaitMessage(fmt.Sprintf("❌ Bot @%s is already registered.", utils.EscapeMarkdown(botInfo.Username)))
				return fmt.Errorf("bot already exists")
			}
		}
		s.logger.Debug("No duplicate bot found",
			zap.Int64("user_id", userID),
			zap.String("bot_username", botInfo.Username))
	} else {
		s.logger.Debug("Failed to get all bots for duplicate check, continuing",
			zap.Int64("user_id", userID),
			zap.Error(err))
	}

	// Encrypt token
	s.logger.Debug("Encrypting bot token",
		zap.Int64("user_id", userID),
		zap.String("bot_username", botInfo.Username))
	encryptedToken, err := utils.EncryptToken(token, s.encryptionKey)
	if err != nil {
		s.logger.Error("Failed to encrypt token", zap.Error(err))
		updateWaitMessage("❌ An error occurred. Please try again later.")
		return err
	}
	s.logger.Debug("Bot token encrypted successfully",
		zap.Int64("user_id", userID),
		zap.String("bot_username", botInfo.Username),
		zap.Int("encrypted_length", len(encryptedToken)))

	// Create bot with transaction to ensure data consistency
	forwarderBot := &models.ForwarderBot{
		Token:     encryptedToken,
		Name:      botInfo.Username,
		ManagerID: user.ID,
	}

	s.logger.Debug("Starting transaction for bot creation",
		zap.Int64("user_id", userID),
		zap.String("bot_username", botInfo.Username),
		zap.String("manager_id", user.ID.String()))

	// Use transaction to ensure atomicity of bot creation, recipient creation, and audit logging
	err = s.db.Transaction(func(tx *gorm.DB) error {
		// Create transaction-aware repositories
		txBotRepo := s.botRepo.WithTx(tx)
		txRecipientRepo := s.recipientRepo.WithTx(tx)
		txAuditRepo := s.auditLogRepo.WithTx(tx)

		// 1. Create bot
		s.logger.Debug("Creating ForwarderBot record in transaction",
			zap.Int64("user_id", userID),
			zap.String("bot_username", botInfo.Username))
		if err := txBotRepo.Create(forwarderBot); err != nil {
			s.logger.Error("Failed to create bot in transaction", zap.Error(err))
			return fmt.Errorf("failed to create bot: %w", err)
		}

		s.logger.Debug("ForwarderBot created successfully in transaction",
			zap.Int64("user_id", userID),
			zap.String("bot_id", forwarderBot.ID.String()),
			zap.String("bot_username", forwarderBot.Name))

		// 2. Add manager as recipient automatically
		s.logger.Debug("Adding manager as recipient in transaction",
			zap.Int64("user_id", userID),
			zap.String("bot_id", forwarderBot.ID.String()),
			zap.Int64("manager_telegram_user_id", user.TelegramUserID))

		// Check if recipient already exists (using transaction-aware repo)
		existingRecipient, err := txRecipientRepo.GetByBotIDAndChatID(forwarderBot.ID, user.TelegramUserID)
		if err == nil && existingRecipient != nil {
			s.logger.Debug("Manager is already a recipient, skipping",
				zap.Int64("user_id", userID),
				zap.String("bot_id", forwarderBot.ID.String()))
		} else {
			// Create recipient for manager
			recipient := &models.Recipient{
				BotID:         forwarderBot.ID,
				RecipientType: models.RecipientTypeUser,
				ChatID:        user.TelegramUserID,
			}

			if err := txRecipientRepo.Create(recipient); err != nil {
				s.logger.Error("Failed to add manager as recipient in transaction",
					zap.Int64("user_id", userID),
					zap.String("bot_id", forwarderBot.ID.String()),
					zap.Error(err))
				// Return error to rollback bot creation
				return fmt.Errorf("failed to add manager as recipient: %w", err)
			}

			s.logger.Debug("Manager added as recipient successfully in transaction",
				zap.Int64("user_id", userID),
				zap.String("bot_id", forwarderBot.ID.String()),
				zap.String("recipient_id", recipient.ID.String()))
		}

		// 3. Log audit
		s.logger.Debug("Creating audit log in transaction",
			zap.Int64("user_id", userID),
			zap.String("bot_id", forwarderBot.ID.String()))
		details, _ := json.Marshal(map[string]interface{}{
			"bot_id":   forwarderBot.ID.String(),
			"bot_name": forwarderBot.Name,
		})
		auditLog := &models.AuditLog{
			UserID:       &user.ID,
			ActionType:   models.AuditLogActionAddBot,
			ResourceType: "bot",
			ResourceID:   forwarderBot.ID,
			Details:      string(details),
		}
		if err := txAuditRepo.Create(auditLog); err != nil {
			s.logger.Error("Failed to create audit log in transaction",
				zap.Int64("user_id", userID),
				zap.String("bot_id", forwarderBot.ID.String()),
				zap.Error(err))
			return fmt.Errorf("failed to create audit log: %w", err)
		}

		return nil // Transaction committed
	})

	if err != nil {
		s.logger.Error("Transaction failed for bot creation",
			zap.Int64("user_id", userID),
			zap.String("bot_username", botInfo.Username),
			zap.Error(err))
		updateWaitMessage("❌ Failed to register bot due to database error. Please try again later.")
		return err
	}

	s.logger.Debug("Transaction completed successfully",
		zap.Int64("user_id", userID),
		zap.String("bot_id", forwarderBot.ID.String()),
		zap.String("bot_username", forwarderBot.Name))
	s.logger.Debug("Audit log created",
		zap.Int64("user_id", userID),
		zap.String("bot_id", forwarderBot.ID.String()))

	// Start the bot immediately if BotManager is available
	if s.botManager != nil {
		s.logger.Debug("Starting ForwarderBot immediately",
			zap.Int64("user_id", userID),
			zap.String("bot_id", forwarderBot.ID.String()),
			zap.String("bot_username", forwarderBot.Name))
		if startErr := s.botManager.StartBot(forwarderBot.ID); startErr != nil {
			s.logger.Error("Failed to start ForwarderBot immediately",
				zap.Int64("user_id", userID),
				zap.String("bot_id", forwarderBot.ID.String()),
				zap.Error(startErr))
			// Continue anyway - bot will be started on next restart
			updateWaitMessage(fmt.Sprintf("⚠️ Bot @%s has been registered, but failed to start immediately. It will be started on next application restart.", utils.EscapeMarkdown(forwarderBot.Name)))
			return startErr
		}
		s.logger.Debug("ForwarderBot started successfully",
			zap.Int64("user_id", userID),
			zap.String("bot_id", forwarderBot.ID.String()),
			zap.String("bot_username", forwarderBot.Name))
	} else {
		s.logger.Debug("BotManager not available, bot will be started on next restart",
			zap.Int64("user_id", userID),
			zap.String("bot_id", forwarderBot.ID.String()))
	}

	s.logger.Debug("Updating wait message to success message",
		zap.Int64("user_id", userID),
		zap.String("bot_username", forwarderBot.Name))
	updateWaitMessage(fmt.Sprintf("✅ Bot @%s has been successfully registered and started!", utils.EscapeMarkdown(forwarderBot.Name)))
	s.logger.Debug("Success message updated",
		zap.Int64("user_id", userID),
		zap.String("bot_username", forwarderBot.Name))
	return nil
}

func (s *Service) handleMyBots(ctx context.Context, b *gotgbot.Bot, update *ext.Context) error {
	userID := update.EffectiveUser.Id
	chatID := update.EffectiveChat.Id
	username := update.EffectiveUser.Username
	var usernamePtr *string
	if username != "" {
		usernamePtr = &username
	}

	s.logger.Debug("Processing /mybots command",
		zap.Int64("user_id", userID),
		zap.Int64("chat_id", chatID),
		zap.String("username", username))

	s.logger.Debug("Getting or creating user",
		zap.Int64("user_id", userID))
	user, err := s.userRepo.GetOrCreateByTelegramUserID(
		update.EffectiveUser.Id,
		usernamePtr)
	if err != nil {
		s.logger.Error("Failed to get or create user", zap.Error(err))
		_, err := b.SendMessage(update.EffectiveChat.Id,
			"An error occurred. Please try again later.", nil)
		return err
	}
	s.logger.Debug("User retrieved/created",
		zap.Int64("user_id", userID),
		zap.String("user_uuid", user.ID.String()))

	s.logger.Debug("Retrieving bots for manager",
		zap.Int64("user_id", userID),
		zap.String("manager_id", user.ID.String()))
	bots, err := s.botRepo.GetByManagerID(user.ID)
	if err != nil {
		s.logger.Error("Failed to get bots", zap.Error(err))
		_, err := b.SendMessage(update.EffectiveChat.Id,
			"An error occurred. Please try again later.", nil)
		return err
	}

	s.logger.Debug("Bots retrieved",
		zap.Int64("user_id", userID),
		zap.Int("bot_count", len(bots)))

	if len(bots) == 0 {
		s.logger.Debug("No bots found for manager",
			zap.Int64("user_id", userID))
		_, err := b.SendMessage(update.EffectiveChat.Id,
			"You don't have any bots registered. Use /addbot to register one.", nil)
		return err
	}

	s.logger.Debug("Building bot list buttons",
		zap.Int64("user_id", userID),
		zap.Int("bot_count", len(bots)))
	var buttons [][]gotgbot.InlineKeyboardButton
	for i, bot := range bots {
		callbackData := fmt.Sprintf("bot:view:%s", bot.ID.String())
		s.logger.Debug("Adding bot button",
			zap.Int64("user_id", userID),
			zap.Int("index", i),
			zap.String("bot_name", bot.Name),
			zap.String("bot_id", bot.ID.String()),
			zap.String("callback_data", callbackData))
		buttons = append(buttons, []gotgbot.InlineKeyboardButton{
			{
				Text:         fmt.Sprintf("@%s", bot.Name),
				CallbackData: callbackData,
			},
		})
	}

	s.logger.Debug("Sending bot list message",
		zap.Int64("user_id", userID),
		zap.Int("button_count", len(buttons)))
	keyboard := gotgbot.InlineKeyboardMarkup{InlineKeyboard: buttons}
	_, err = b.SendMessage(update.EffectiveChat.Id,
		"Select a bot to manage:", &gotgbot.SendMessageOpts{
			ReplyMarkup: keyboard,
		})
	if err != nil {
		s.logger.Debug("Failed to send bot list message",
			zap.Int64("user_id", userID),
			zap.Error(err))
	} else {
		s.logger.Debug("Bot list message sent successfully",
			zap.Int64("user_id", userID),
			zap.Int("bot_count", len(bots)))
	}
	return err
}

func (s *Service) handleStats(ctx context.Context, b *gotgbot.Bot, update *ext.Context) error {
	userID := update.EffectiveUser.Id
	chatID := update.EffectiveChat.Id

	s.logger.Debug("Processing /stats command",
		zap.Int64("user_id", userID),
		zap.Int64("chat_id", chatID))

	s.logger.Debug("Retrieving global statistics",
		zap.Int64("user_id", userID))
	stats, err := s.statsService.GetGlobalStatistics()
	if err != nil {
		s.logger.Error("Failed to get statistics", zap.Error(err))
		_, err := b.SendMessage(update.EffectiveChat.Id,
			"Failed to retrieve statistics. Please try again later.", nil)
		return err
	}

	s.logger.Debug("Global statistics retrieved",
		zap.Int64("user_id", userID),
		zap.Int64("manager_count", stats.ManagerCount),
		zap.Int64("bot_count", stats.BotCount),
		zap.Int64("total_inbound", stats.TotalInbound),
		zap.Int64("total_outbound", stats.TotalOutbound),
		zap.Int64("total_guests", stats.TotalGuestCount))

	message := fmt.Sprintf(
		"*Global Statistics*\n\n"+
			"Managers: %d\n"+
			"Bots: %d\n"+
			"Inbound Messages: %d\n"+
			"Outbound Messages: %d\n"+
			"Total Guests: %d",
		stats.ManagerCount,
		stats.BotCount,
		stats.TotalInbound,
		stats.TotalOutbound,
		stats.TotalGuestCount,
	)

	s.logger.Debug("Sending statistics message",
		zap.Int64("user_id", userID),
		zap.Int64("chat_id", chatID))
	_, err = b.SendMessage(update.EffectiveChat.Id, message, &gotgbot.SendMessageOpts{
		ParseMode: "Markdown",
	})
	if err != nil {
		s.logger.Debug("Failed to send statistics message",
			zap.Int64("user_id", userID),
			zap.Error(err))
	} else {
		s.logger.Debug("Statistics message sent successfully",
			zap.Int64("user_id", userID))
	}
	return err
}

func (s *Service) handleManage(ctx context.Context, b *gotgbot.Bot, update *ext.Context) error {
	userID := update.EffectiveUser.Id
	chatID := update.EffectiveChat.Id

	s.logger.Debug("Processing /manage command",
		zap.Int64("user_id", userID),
		zap.Int64("chat_id", chatID))

	s.logger.Debug("Building management menu buttons",
		zap.Int64("user_id", userID))
	buttons := [][]gotgbot.InlineKeyboardButton{
		{
			{Text: "View All Bots", CallbackData: "manage:all_bots"},
		},
		{
			{Text: "View All Managers", CallbackData: "manage:all_managers"},
		},
	}

	s.logger.Debug("Sending management menu",
		zap.Int64("user_id", userID),
		zap.Int64("chat_id", chatID))
	keyboard := gotgbot.InlineKeyboardMarkup{InlineKeyboard: buttons}
	_, err := b.SendMessage(update.EffectiveChat.Id,
		"Management Menu:", &gotgbot.SendMessageOpts{
			ReplyMarkup: keyboard,
		})
	if err != nil {
		s.logger.Debug("Failed to send management menu",
			zap.Int64("user_id", userID),
			zap.Error(err))
	} else {
		s.logger.Debug("Management menu sent successfully",
			zap.Int64("user_id", userID))
	}
	return err
}

func (s *Service) handleHelp(ctx context.Context, b *gotgbot.Bot, update *ext.Context) error {
	userID := update.EffectiveUser.Id
	chatID := update.EffectiveChat.Id

	s.logger.Debug("Processing /help command",
		zap.Int64("user_id", userID),
		zap.Int64("chat_id", chatID))

	isSuperuser := s.IsSuperuser(userID)
	s.logger.Debug("Building help message",
		zap.Int64("user_id", userID),
		zap.Bool("is_superuser", isSuperuser))

	helpText := "*ManagerBot Commands*\n\n"
	helpText += "*/help* - Show this help message\n"
	helpText += "*/addbot <token>* - Register a new ForwarderBot\n"
	helpText += "*/mybots* - List all your ForwarderBots\n"

	if isSuperuser {
		helpText += "\n*Superuser Commands:*\n"
		helpText += "*/manage* - Open management menu\n"
		helpText += "*/stats* - View global statistics\n"
	}

	helpText += "\n*Usage:*\n"
	helpText += "1. Use /addbot to register a ForwarderBot\n"
	helpText += "2. Use /mybots to manage your bots\n"
	helpText += "3. Each ForwarderBot can forward messages between Guests and Recipients"

	s.logger.Debug("Sending help message",
		zap.Int64("user_id", userID),
		zap.Int64("chat_id", chatID),
		zap.Int("message_length", len(helpText)))
	_, err := b.SendMessage(update.EffectiveChat.Id, helpText, &gotgbot.SendMessageOpts{
		ParseMode: "Markdown",
	})
	if err != nil {
		s.logger.Debug("Failed to send help message",
			zap.Int64("user_id", userID),
			zap.Error(err))
	} else {
		s.logger.Debug("Help message sent successfully",
			zap.Int64("user_id", userID))
	}
	return err
}
