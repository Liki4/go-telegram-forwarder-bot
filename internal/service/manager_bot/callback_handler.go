package manager_bot

import (
	"context"
	"encoding/json"
	"fmt"

	"go-telegram-forwarder-bot/internal/models"
	"go-telegram-forwarder-bot/internal/utils"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// getMessageIDFromCallback safely extracts MessageId from MaybeInaccessibleMessage
// MaybeInaccessibleMessage is an interface with GetMessageId() method
func getMessageIDFromCallback(msg gotgbot.MaybeInaccessibleMessage) (int64, error) {
	if msg == nil {
		return 0, fmt.Errorf("message is nil")
	}

	// MaybeInaccessibleMessage interface has GetMessageId() method
	return msg.GetMessageId(), nil
}

func (s *Service) handleManageCallback(ctx context.Context, b *gotgbot.Bot, update *ext.Context, parts []string) error {
	if len(parts) < 1 {
		_, err := b.AnswerCallbackQuery(update.CallbackQuery.Id, &gotgbot.AnswerCallbackQueryOpts{
			Text: "Invalid callback data",
		})
		return err
	}

	action := parts[0]
	switch action {
	case "menu":
		return s.handleManageMenu(ctx, b, update)
	case "all_bots":
		return s.handleAllBots(ctx, b, update)
	case "all_managers":
		return s.handleAllManagers(ctx, b, update)
	case "bot":
		if len(parts) < 2 {
			_, err := b.AnswerCallbackQuery(update.CallbackQuery.Id, &gotgbot.AnswerCallbackQueryOpts{
				Text: "Invalid callback data",
			})
			return err
		}
		botID, err := uuid.Parse(parts[1])
		if err != nil {
			_, err := b.AnswerCallbackQuery(update.CallbackQuery.Id, &gotgbot.AnswerCallbackQueryOpts{
				Text: "Invalid bot ID",
			})
			return err
		}
		return s.handleViewBot(ctx, b, update, botID)
	default:
		_, err := b.AnswerCallbackQuery(update.CallbackQuery.Id, &gotgbot.AnswerCallbackQueryOpts{
			Text: "Unknown action",
		})
		return err
	}
}

func (s *Service) handleBotCallback(ctx context.Context, b *gotgbot.Bot, update *ext.Context, parts []string) error {
	userID := update.EffectiveUser.Id

	if len(parts) < 2 {
		_, err := b.AnswerCallbackQuery(update.CallbackQuery.Id, &gotgbot.AnswerCallbackQueryOpts{
			Text: "Invalid callback data",
		})
		return err
	}

	action := parts[0]
	botID, err := uuid.Parse(parts[1])
	if err != nil {
		_, err := b.AnswerCallbackQuery(update.CallbackQuery.Id, &gotgbot.AnswerCallbackQueryOpts{
			Text: "Invalid bot ID",
		})
		return err
	}

	// Check permissions for bot operations
	// Superusers have all permissions, so check that first
	isSuperuser := s.IsSuperuser(userID)
	if !isSuperuser {
		// For non-superusers, check if they are the bot's manager
		isManager, err := s.IsBotManager(userID, botID)
		if err != nil {
			s.logger.Warn("Failed to check bot manager status", zap.Error(err))
			_, err := b.AnswerCallbackQuery(update.CallbackQuery.Id, &gotgbot.AnswerCallbackQueryOpts{
				Text: "Failed to verify permissions",
			})
			return err
		}
		if !isManager {
			s.logger.Debug("Access denied for bot callback",
				zap.Int64("user_id", userID),
				zap.String("bot_id", botID.String()),
				zap.String("action", action))
			_, err := b.AnswerCallbackQuery(update.CallbackQuery.Id, &gotgbot.AnswerCallbackQueryOpts{
				Text: "You are not authorized to access this bot.",
			})
			return err
		}
	}

	switch action {
	case "view":
		return s.handleViewBot(ctx, b, update, botID)
	case "delete":
		return s.handleConfirmDeleteBot(ctx, b, update, botID)
	default:
		_, err := b.AnswerCallbackQuery(update.CallbackQuery.Id, &gotgbot.AnswerCallbackQueryOpts{
			Text: "Unknown action",
		})
		return err
	}
}

func (s *Service) handleDeleteBotCallback(ctx context.Context, b *gotgbot.Bot, update *ext.Context, parts []string) error {
	userID := update.EffectiveUser.Id

	if len(parts) < 2 {
		return fmt.Errorf("invalid callback data")
	}

	action := parts[0]
	botID, err := uuid.Parse(parts[1])
	if err != nil {
		_, err := b.AnswerCallbackQuery(update.CallbackQuery.Id, &gotgbot.AnswerCallbackQueryOpts{
			Text: "Invalid bot ID",
		})
		return err
	}

	// Check permissions: Superusers have all permissions, so check that first
	isSuperuser := s.IsSuperuser(userID)
	if !isSuperuser {
		// For non-superusers, check if they are the bot's manager
		isManager, err := s.IsBotManager(userID, botID)
		if err != nil {
			s.logger.Warn("Failed to check bot manager status", zap.Error(err))
			_, err := b.AnswerCallbackQuery(update.CallbackQuery.Id, &gotgbot.AnswerCallbackQueryOpts{
				Text: "Failed to verify permissions",
			})
			return err
		}
		if !isManager {
			s.logger.Debug("Access denied for delete bot",
				zap.Int64("user_id", userID),
				zap.String("bot_id", botID.String()))
			_, err := b.AnswerCallbackQuery(update.CallbackQuery.Id, &gotgbot.AnswerCallbackQueryOpts{
				Text: "You are not authorized to delete this bot.",
			})
			return err
		}
	}

	switch action {
	case "confirm":
		// Show confirmation dialog
		return s.handleConfirmDeleteBot(ctx, b, update, botID)
	case "yes":
		// Execute deletion
		return s.executeDeleteBot(ctx, b, update, botID)
	case "no":
		// User cancelled
		_, err := b.AnswerCallbackQuery(update.CallbackQuery.Id, &gotgbot.AnswerCallbackQueryOpts{
			Text: "Deletion cancelled",
		})
		return err
	default:
		_, err := b.AnswerCallbackQuery(update.CallbackQuery.Id, &gotgbot.AnswerCallbackQueryOpts{
			Text: "Unknown action",
		})
		return err
	}
}

func (s *Service) executeDeleteBot(ctx context.Context, b *gotgbot.Bot, update *ext.Context, botID uuid.UUID) error {
	// Answer callback query first
	_, err := b.AnswerCallbackQuery(update.CallbackQuery.Id, &gotgbot.AnswerCallbackQueryOpts{})
	if err != nil {
		s.logger.Warn("Failed to answer callback query", zap.Error(err))
	}

	bot, err := s.botRepo.GetByID(botID)
	if err != nil {
		_, err := b.AnswerCallbackQuery(update.CallbackQuery.Id, &gotgbot.AnswerCallbackQueryOpts{
			Text: "Failed to load bot information",
		})
		return err
	}

	// Stop the bot immediately if BotManager is available
	if s.botManager != nil {
		s.logger.Debug("Stopping ForwarderBot immediately",
			zap.String("bot_id", botID.String()),
			zap.String("bot_name", bot.Name))
		if stopErr := s.botManager.StopBot(botID); stopErr != nil {
			s.logger.Warn("Failed to stop ForwarderBot immediately",
				zap.String("bot_id", botID.String()),
				zap.Error(stopErr))
			// Continue with deletion anyway
		} else {
			s.logger.Debug("ForwarderBot stopped successfully",
				zap.String("bot_id", botID.String()),
				zap.String("bot_name", bot.Name))
		}
	}

	if err := s.botRepo.Delete(botID); err != nil {
		s.logger.Error("Failed to delete bot", zap.Error(err))
		_, err := b.AnswerCallbackQuery(update.CallbackQuery.Id, &gotgbot.AnswerCallbackQueryOpts{
			Text: "Failed to delete bot",
		})
		return err
	}

	// Log audit
	userID := update.EffectiveUser.Id
	user, _ := s.userRepo.GetByTelegramUserID(userID)
	if user != nil {
		details, _ := json.Marshal(map[string]interface{}{
			"bot_id":   bot.ID.String(),
			"bot_name": bot.Name,
		})
		auditLog := &models.AuditLog{
			UserID:       &user.ID,
			ActionType:   models.AuditLogActionDeleteBot,
			ResourceType: "bot",
			ResourceID:   bot.ID,
			Details:      string(details),
		}
		s.auditLogRepo.Create(auditLog)
	}

	messageID, err := getMessageIDFromCallback(update.CallbackQuery.Message)
	if err != nil {
		s.logger.Warn("Failed to get message ID from callback", zap.Error(err))
		_, err := b.AnswerCallbackQuery(update.CallbackQuery.Id, &gotgbot.AnswerCallbackQueryOpts{
			Text: "Failed to get message ID",
		})
		return err
	}
	_, _, err = b.EditMessageText(fmt.Sprintf("Bot @%s has been deleted.", utils.EscapeMarkdown(bot.Name)),
		&gotgbot.EditMessageTextOpts{
			ChatId:    update.EffectiveChat.Id,
			MessageId: messageID,
			ParseMode: "Markdown",
		})
	return err
}

func (s *Service) handleManageMenu(ctx context.Context, b *gotgbot.Bot, update *ext.Context) error {
	userID := update.EffectiveUser.Id

	// Only superusers can access this
	if !s.IsSuperuser(userID) {
		s.logger.Debug("Access denied for manage menu",
			zap.Int64("user_id", userID))
		_, err := b.AnswerCallbackQuery(update.CallbackQuery.Id, &gotgbot.AnswerCallbackQueryOpts{
			Text: "You are not authorized to access this.",
		})
		return err
	}

	// Answer callback query first
	_, err := b.AnswerCallbackQuery(update.CallbackQuery.Id, &gotgbot.AnswerCallbackQueryOpts{})
	if err != nil {
		s.logger.Warn("Failed to answer callback query", zap.Error(err))
	}

	buttons := [][]gotgbot.InlineKeyboardButton{
		{
			{Text: "View All Bots", CallbackData: "manage:all_bots"},
		},
		{
			{Text: "View All Managers", CallbackData: "manage:all_managers"},
		},
	}

	messageID, err := getMessageIDFromCallback(update.CallbackQuery.Message)
	if err != nil {
		s.logger.Warn("Failed to get message ID from callback", zap.Error(err))
		// Try to send a new message if we can't get message ID
		keyboard := gotgbot.InlineKeyboardMarkup{InlineKeyboard: buttons}
		_, sendErr := b.SendMessage(update.EffectiveChat.Id, "Management Menu:", &gotgbot.SendMessageOpts{
			ReplyMarkup: keyboard,
		})
		return sendErr
	}
	keyboard := gotgbot.InlineKeyboardMarkup{InlineKeyboard: buttons}
	_, _, err = b.EditMessageText("Management Menu:", &gotgbot.EditMessageTextOpts{
		ChatId:      update.EffectiveChat.Id,
		MessageId:   messageID,
		ReplyMarkup: keyboard,
	})
	if err != nil {
		s.logger.Error("Failed to edit message", zap.Error(err))
		// Try to send a new message if edit fails
		_, sendErr := b.SendMessage(update.EffectiveChat.Id, "Management Menu:", &gotgbot.SendMessageOpts{
			ReplyMarkup: keyboard,
		})
		return sendErr
	}
	return nil
}

func (s *Service) handleAllBots(ctx context.Context, b *gotgbot.Bot, update *ext.Context) error {
	userID := update.EffectiveUser.Id

	// Only superusers can access this
	if !s.IsSuperuser(userID) {
		s.logger.Debug("Access denied for all_bots",
			zap.Int64("user_id", userID))
		_, err := b.AnswerCallbackQuery(update.CallbackQuery.Id, &gotgbot.AnswerCallbackQueryOpts{
			Text: "You are not authorized to access this.",
		})
		return err
	}

	// Answer callback query first
	_, err := b.AnswerCallbackQuery(update.CallbackQuery.Id, &gotgbot.AnswerCallbackQueryOpts{})
	if err != nil {
		s.logger.Warn("Failed to answer callback query", zap.Error(err))
	}

	bots, err := s.botRepo.GetAll()
	if err != nil {
		_, err := b.AnswerCallbackQuery(update.CallbackQuery.Id, &gotgbot.AnswerCallbackQueryOpts{
			Text: "Failed to load bots",
		})
		return err
	}

	if len(bots) == 0 {
		_, err := b.AnswerCallbackQuery(update.CallbackQuery.Id, &gotgbot.AnswerCallbackQueryOpts{
			Text: "No bots registered",
		})
		return err
	}

	var buttons [][]gotgbot.InlineKeyboardButton
	for _, bot := range bots {
		buttons = append(buttons, []gotgbot.InlineKeyboardButton{
			{
				Text:         fmt.Sprintf("@%s", bot.Name),
				CallbackData: fmt.Sprintf("bot:view:%s", bot.ID.String()),
			},
		})
	}

	// Add Back button to return to manage menu
	buttons = append(buttons, []gotgbot.InlineKeyboardButton{
		{
			Text:         "Back",
			CallbackData: "manage:menu",
		},
	})

	messageID, err := getMessageIDFromCallback(update.CallbackQuery.Message)
	if err != nil {
		s.logger.Warn("Failed to get message ID from callback", zap.Error(err))
		_, err := b.AnswerCallbackQuery(update.CallbackQuery.Id, &gotgbot.AnswerCallbackQueryOpts{
			Text: "Failed to get message ID",
		})
		return err
	}
	keyboard := gotgbot.InlineKeyboardMarkup{InlineKeyboard: buttons}
	_, _, err = b.EditMessageText("Select a bot to view details:",
		&gotgbot.EditMessageTextOpts{
			ChatId:      update.EffectiveChat.Id,
			MessageId:   messageID,
			ReplyMarkup: keyboard,
		})
	return err
}

func (s *Service) handleAllManagers(ctx context.Context, b *gotgbot.Bot, update *ext.Context) error {
	// Answer callback query first
	_, err := b.AnswerCallbackQuery(update.CallbackQuery.Id, &gotgbot.AnswerCallbackQueryOpts{})
	if err != nil {
		s.logger.Warn("Failed to answer callback query", zap.Error(err))
	}

	bots, err := s.botRepo.GetAll()
	if err != nil {
		_, err := b.AnswerCallbackQuery(update.CallbackQuery.Id, &gotgbot.AnswerCallbackQueryOpts{
			Text: "Failed to load managers",
		})
		return err
	}

	managerMap := make(map[uuid.UUID]*models.User)
	for _, bot := range bots {
		if _, exists := managerMap[bot.ManagerID]; !exists {
			manager, err := s.userRepo.GetByID(bot.ManagerID)
			if err == nil {
				managerMap[bot.ManagerID] = manager
			}
		}
	}

	if len(managerMap) == 0 {
		_, err := b.AnswerCallbackQuery(update.CallbackQuery.Id, &gotgbot.AnswerCallbackQueryOpts{
			Text: "No managers found",
		})
		return err
	}

	var buttons [][]gotgbot.InlineKeyboardButton
	for _, manager := range managerMap {
		username := "Unknown"
		if manager.Username != nil {
			username = *manager.Username
		}
		buttons = append(buttons, []gotgbot.InlineKeyboardButton{
			{
				Text:         fmt.Sprintf("@%s", username),
				CallbackData: fmt.Sprintf("manager:view:%s", manager.ID.String()),
			},
		})
	}

	// Add Back button to return to manage menu
	buttons = append(buttons, []gotgbot.InlineKeyboardButton{
		{
			Text:         "Back",
			CallbackData: "manage:menu",
		},
	})

	messageID, err := getMessageIDFromCallback(update.CallbackQuery.Message)
	if err != nil {
		s.logger.Warn("Failed to get message ID from callback", zap.Error(err))
		_, err := b.AnswerCallbackQuery(update.CallbackQuery.Id, &gotgbot.AnswerCallbackQueryOpts{
			Text: "Failed to get message ID",
		})
		return err
	}
	keyboard := gotgbot.InlineKeyboardMarkup{InlineKeyboard: buttons}
	_, _, err = b.EditMessageText("Select a manager to view their bots:",
		&gotgbot.EditMessageTextOpts{
			ChatId:      update.EffectiveChat.Id,
			MessageId:   messageID,
			ReplyMarkup: keyboard,
		})
	return err
}

func (s *Service) handleManagerCallback(ctx context.Context, b *gotgbot.Bot, update *ext.Context, parts []string) error {
	if len(parts) < 2 {
		_, err := b.AnswerCallbackQuery(update.CallbackQuery.Id, &gotgbot.AnswerCallbackQueryOpts{
			Text: "Invalid callback data",
		})
		return err
	}

	action := parts[0]
	managerID, err := uuid.Parse(parts[1])
	if err != nil {
		_, err := b.AnswerCallbackQuery(update.CallbackQuery.Id, &gotgbot.AnswerCallbackQueryOpts{
			Text: "Invalid manager ID",
		})
		return err
	}

	switch action {
	case "view":
		return s.handleViewManager(ctx, b, update, managerID)
	default:
		_, err := b.AnswerCallbackQuery(update.CallbackQuery.Id, &gotgbot.AnswerCallbackQueryOpts{
			Text: "Unknown action",
		})
		return err
	}
}

func (s *Service) handleViewManager(ctx context.Context, b *gotgbot.Bot, update *ext.Context, managerID uuid.UUID) error {
	// Answer callback query first
	_, err := b.AnswerCallbackQuery(update.CallbackQuery.Id, &gotgbot.AnswerCallbackQueryOpts{})
	if err != nil {
		s.logger.Warn("Failed to answer callback query", zap.Error(err))
	}

	manager, err := s.userRepo.GetByID(managerID)
	if err != nil {
		_, err := b.AnswerCallbackQuery(update.CallbackQuery.Id, &gotgbot.AnswerCallbackQueryOpts{
			Text: "Failed to load manager information",
		})
		return err
	}

	bots, err := s.botRepo.GetByManagerID(managerID)
	if err != nil {
		_, err := b.AnswerCallbackQuery(update.CallbackQuery.Id, &gotgbot.AnswerCallbackQueryOpts{
			Text: "Failed to load manager's bots",
		})
		return err
	}

	stats, err := s.statsService.GetManagerStatistics(managerID)
	if err != nil {
		s.logger.Warn("Failed to get manager statistics", zap.Error(err))
	}

	username := "Unknown"
	if manager.Username != nil {
		username = *manager.Username
	}

	message := fmt.Sprintf(
		"*Manager Information*\n\n"+
			"Username: @%s\n"+
			"Telegram User ID: %d\n"+
			"Total Bots: %d",
		utils.EscapeMarkdown(username),
		manager.TelegramUserID,
		len(bots),
	)

	if stats != nil && len(stats.Bots) > 0 {
		totalInbound := int64(0)
		totalOutbound := int64(0)
		totalGuests := int64(0)
		for _, botStat := range stats.Bots {
			totalInbound += botStat.InboundCount
			totalOutbound += botStat.OutboundCount
			totalGuests += botStat.GuestCount
		}
		message += fmt.Sprintf(
			"\n\n*Statistics*\n"+
				"Total Inbound: %d\n"+
				"Total Outbound: %d\n"+
				"Total Guests: %d",
			totalInbound,
			totalOutbound,
			totalGuests,
		)
	}

	var buttons [][]gotgbot.InlineKeyboardButton
	if len(bots) > 0 {
		for _, bot := range bots {
			buttons = append(buttons, []gotgbot.InlineKeyboardButton{
				{
					Text:         fmt.Sprintf("@%s", bot.Name),
					CallbackData: fmt.Sprintf("bot:view:%s", bot.ID.String()),
				},
			})
		}
	}

	// Add Back button
	buttons = append(buttons, []gotgbot.InlineKeyboardButton{
		{
			Text:         "Back",
			CallbackData: "manage:all_managers",
		},
	})

	messageID, err := getMessageIDFromCallback(update.CallbackQuery.Message)
	if err != nil {
		s.logger.Warn("Failed to get message ID from callback", zap.Error(err))
		// Try to send a new message if we can't get message ID
		keyboard := gotgbot.InlineKeyboardMarkup{InlineKeyboard: buttons}
		_, sendErr := b.SendMessage(update.EffectiveChat.Id, message, &gotgbot.SendMessageOpts{
			ParseMode:   "Markdown",
			ReplyMarkup: keyboard,
		})
		return sendErr
	}
	keyboard := gotgbot.InlineKeyboardMarkup{InlineKeyboard: buttons}
	_, _, err = b.EditMessageText(message, &gotgbot.EditMessageTextOpts{
		ChatId:      update.EffectiveChat.Id,
		MessageId:   messageID,
		ParseMode:   "Markdown",
		ReplyMarkup: keyboard,
	})
	if err != nil {
		s.logger.Error("Failed to edit message", zap.Error(err))
		// Try to send a new message if edit fails
		_, sendErr := b.SendMessage(update.EffectiveChat.Id, message, &gotgbot.SendMessageOpts{
			ParseMode:   "Markdown",
			ReplyMarkup: keyboard,
		})
		return sendErr
	}
	return nil
}

func (s *Service) handleViewBot(ctx context.Context, b *gotgbot.Bot, update *ext.Context, botID uuid.UUID) error {
	userID := update.EffectiveUser.Id

	// Check permissions: Superusers have all permissions, so check that first
	isSuperuser := s.IsSuperuser(userID)
	var isManager bool
	if !isSuperuser {
		// For non-superusers, check if they are the bot's manager
		var err error
		isManager, err = s.IsBotManager(userID, botID)
		if err != nil {
			s.logger.Warn("Failed to check bot manager status", zap.Error(err))
			_, err := b.AnswerCallbackQuery(update.CallbackQuery.Id, &gotgbot.AnswerCallbackQueryOpts{
				Text: "Failed to verify permissions",
			})
			return err
		}
		if !isManager {
			s.logger.Debug("Access denied for bot view",
				zap.Int64("user_id", userID),
				zap.String("bot_id", botID.String()))
			_, err := b.AnswerCallbackQuery(update.CallbackQuery.Id, &gotgbot.AnswerCallbackQueryOpts{
				Text: "You are not authorized to view this bot.",
			})
			return err
		}
	}

	// Answer callback query first
	_, err := b.AnswerCallbackQuery(update.CallbackQuery.Id, &gotgbot.AnswerCallbackQueryOpts{})
	if err != nil {
		s.logger.Warn("Failed to answer callback query", zap.Error(err))
	}

	bot, err := s.botRepo.GetByID(botID)
	if err != nil {
		_, err := b.AnswerCallbackQuery(update.CallbackQuery.Id, &gotgbot.AnswerCallbackQueryOpts{
			Text: "Failed to load bot information",
		})
		return err
	}

	stats, err := s.statsService.GetBotStatistics(botID)
	if err != nil {
		s.logger.Warn("Failed to get bot statistics", zap.Error(err))
	}

	message := fmt.Sprintf(
		"*Bot Information*\n\n"+
			"Name: @%s\n"+
			"Manager ID: %d\n"+
			"Created: %s",
		utils.EscapeMarkdown(bot.Name),
		bot.Manager.TelegramUserID,
		bot.CreatedAt.Format("2006-01-02 15:04:05"),
	)

	if stats != nil {
		message += fmt.Sprintf(
			"\n\n*Statistics*\n"+
				"Inbound: %d\n"+
				"Outbound: %d\n"+
				"Guests: %d",
			stats.InboundCount,
			stats.OutboundCount,
			stats.GuestCount,
		)
	}

	// Only show Delete Bot button if user is the manager or superuser
	buttons := [][]gotgbot.InlineKeyboardButton{}
	if isManager || isSuperuser {
		buttons = append(buttons, []gotgbot.InlineKeyboardButton{
			{
				Text:         "Delete Bot",
				CallbackData: fmt.Sprintf("delete_bot:confirm:%s", botID.String()),
			},
		})
	}

	// Back button behavior depends on user role
	var backCallbackData string
	if isSuperuser {
		// Superuser: back to manage:all_bots
		backCallbackData = "manage:all_bots"
	} else {
		// Regular manager: back to mybots list
		backCallbackData = "mybots:list"
	}
	buttons = append(buttons, []gotgbot.InlineKeyboardButton{
		{
			Text:         "Back",
			CallbackData: backCallbackData,
		},
	})

	messageID, err := getMessageIDFromCallback(update.CallbackQuery.Message)
	if err != nil {
		s.logger.Warn("Failed to get message ID from callback", zap.Error(err))
		// Try to send a new message if we can't get message ID
		keyboard := gotgbot.InlineKeyboardMarkup{InlineKeyboard: buttons}
		_, sendErr := b.SendMessage(update.EffectiveChat.Id, message, &gotgbot.SendMessageOpts{
			ParseMode:   "Markdown",
			ReplyMarkup: keyboard,
		})
		return sendErr
	}
	keyboard := gotgbot.InlineKeyboardMarkup{InlineKeyboard: buttons}
	_, _, err = b.EditMessageText(message, &gotgbot.EditMessageTextOpts{
		ChatId:      update.EffectiveChat.Id,
		MessageId:   messageID,
		ParseMode:   "Markdown",
		ReplyMarkup: keyboard,
	})
	if err != nil {
		s.logger.Error("Failed to edit message", zap.Error(err))
		// Try to send a new message if edit fails
		_, sendErr := b.SendMessage(update.EffectiveChat.Id, message, &gotgbot.SendMessageOpts{
			ParseMode:   "Markdown",
			ReplyMarkup: keyboard,
		})
		return sendErr
	}
	return nil
}

func (s *Service) handleMyBotsCallback(ctx context.Context, b *gotgbot.Bot, update *ext.Context) error {
	userID := update.EffectiveUser.Id

	// Answer callback query first
	_, err := b.AnswerCallbackQuery(update.CallbackQuery.Id, &gotgbot.AnswerCallbackQueryOpts{})
	if err != nil {
		s.logger.Warn("Failed to answer callback query", zap.Error(err))
	}

	// Get or create user
	username := update.EffectiveUser.Username
	var usernamePtr *string
	if username != "" {
		usernamePtr = &username
	}

	user, err := s.userRepo.GetOrCreateByTelegramUserID(userID, usernamePtr)
	if err != nil {
		s.logger.Error("Failed to get or create user", zap.Error(err))
		_, err := b.AnswerCallbackQuery(update.CallbackQuery.Id, &gotgbot.AnswerCallbackQueryOpts{
			Text: "An error occurred. Please try again later.",
		})
		return err
	}

	bots, err := s.botRepo.GetByManagerID(user.ID)
	if err != nil {
		s.logger.Error("Failed to get bots", zap.Error(err))
		_, err := b.AnswerCallbackQuery(update.CallbackQuery.Id, &gotgbot.AnswerCallbackQueryOpts{
			Text: "An error occurred. Please try again later.",
		})
		return err
	}

	if len(bots) == 0 {
		messageID, err := getMessageIDFromCallback(update.CallbackQuery.Message)
		if err != nil {
			s.logger.Warn("Failed to get message ID from callback", zap.Error(err))
			_, err := b.SendMessage(update.EffectiveChat.Id,
				"You don't have any bots registered. Use /addbot to register one.", nil)
			return err
		}
		_, _, err = b.EditMessageText("You don't have any bots registered. Use /addbot to register one.",
			&gotgbot.EditMessageTextOpts{
				ChatId:    update.EffectiveChat.Id,
				MessageId: messageID,
			})
		return err
	}

	var buttons [][]gotgbot.InlineKeyboardButton
	for _, bot := range bots {
		buttons = append(buttons, []gotgbot.InlineKeyboardButton{
			{
				Text:         fmt.Sprintf("@%s", bot.Name),
				CallbackData: fmt.Sprintf("bot:view:%s", bot.ID.String()),
			},
		})
	}

	// No Back button for /mybots list - it's the root level for managers

	messageID, err := getMessageIDFromCallback(update.CallbackQuery.Message)
	if err != nil {
		s.logger.Warn("Failed to get message ID from callback", zap.Error(err))
		// Try to send a new message if we can't get message ID
		keyboard := gotgbot.InlineKeyboardMarkup{InlineKeyboard: buttons}
		_, sendErr := b.SendMessage(update.EffectiveChat.Id, "Select a bot to manage:", &gotgbot.SendMessageOpts{
			ReplyMarkup: keyboard,
		})
		return sendErr
	}
	keyboard := gotgbot.InlineKeyboardMarkup{InlineKeyboard: buttons}
	_, _, err = b.EditMessageText("Select a bot to manage:",
		&gotgbot.EditMessageTextOpts{
			ChatId:      update.EffectiveChat.Id,
			MessageId:   messageID,
			ReplyMarkup: keyboard,
		})
	if err != nil {
		s.logger.Error("Failed to edit message", zap.Error(err))
		// Try to send a new message if edit fails
		_, sendErr := b.SendMessage(update.EffectiveChat.Id, "Select a bot to manage:", &gotgbot.SendMessageOpts{
			ReplyMarkup: keyboard,
		})
		return sendErr
	}
	return nil
}

func (s *Service) handleConfirmDeleteBot(ctx context.Context, b *gotgbot.Bot, update *ext.Context, botID uuid.UUID) error {
	// Answer callback query first
	_, err := b.AnswerCallbackQuery(update.CallbackQuery.Id, &gotgbot.AnswerCallbackQueryOpts{})
	if err != nil {
		s.logger.Warn("Failed to answer callback query", zap.Error(err))
	}

	buttons := [][]gotgbot.InlineKeyboardButton{
		{
			{
				Text:         "Yes, Delete",
				CallbackData: fmt.Sprintf("delete_bot:yes:%s", botID.String()),
			},
			{
				Text:         "Cancel",
				CallbackData: fmt.Sprintf("delete_bot:no:%s", botID.String()),
			},
		},
	}

	messageID, err := getMessageIDFromCallback(update.CallbackQuery.Message)
	if err != nil {
		s.logger.Warn("Failed to get message ID from callback", zap.Error(err))
		_, err := b.AnswerCallbackQuery(update.CallbackQuery.Id, &gotgbot.AnswerCallbackQueryOpts{
			Text: "Failed to get message ID",
		})
		return err
	}
	keyboard := gotgbot.InlineKeyboardMarkup{InlineKeyboard: buttons}
	bot, err := s.botRepo.GetByID(botID)
	if err != nil {
		s.logger.Warn("Failed to get bot for confirmation message", zap.Error(err))
		_, err := b.AnswerCallbackQuery(update.CallbackQuery.Id, &gotgbot.AnswerCallbackQueryOpts{
			Text: "Failed to load bot information",
		})
		return err
	}
	_, _, err = b.EditMessageText(fmt.Sprintf("Are you sure you want to delete bot @%s? This action cannot be undone.", utils.EscapeMarkdown(bot.Name)),
		&gotgbot.EditMessageTextOpts{
			ChatId:      update.EffectiveChat.Id,
			MessageId:   messageID,
			ParseMode:   "Markdown",
			ReplyMarkup: keyboard,
		})
	if err != nil {
		s.logger.Error("Failed to edit message", zap.Error(err))
	}
	return err
}
