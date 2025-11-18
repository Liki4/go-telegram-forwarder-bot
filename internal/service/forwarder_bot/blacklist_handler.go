package forwarder_bot

import (
	"context"
	"encoding/json"
	"fmt"

	"go-telegram-forwarder-bot/internal/models"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

func (s *Service) handleBan(ctx context.Context, b *gotgbot.Bot, update *ext.Context) error {
	if update.EffectiveMessage.ReplyToMessage == nil {
		_, err := b.SendMessage(update.EffectiveChat.Id,
			"Please reply to a message from the user you want to ban.", nil)
		return err
	}

	replyTo := update.EffectiveMessage.ReplyToMessage
	guestUserID := replyTo.From.Id

	// Check if user has permission
	chatID := update.EffectiveChat.Id
	userID := update.EffectiveUser.Id

	// Check if chat is a recipient
	recipient, err := s.recipientRepo.GetByBotIDAndChatID(s.botID, chatID)
	if err != nil {
		_, err := b.SendMessage(update.EffectiveChat.Id,
			"This command can only be used in recipient chats.", nil)
		return err
	}

	// Check permission: Manager, Admin, or any user in recipient chat
	isManagerOrAdmin, err := s.IsManagerOrAdmin(userID)
	if err != nil {
		s.logger.Warn("Failed to check permission", zap.Error(err))
	}
	if !isManagerOrAdmin && recipient.RecipientType != models.RecipientTypeGroup {
		_, err := b.SendMessage(update.EffectiveChat.Id,
			"You are not authorized to use this command.", nil)
		return err
	}

	// Get or create request user
	requestUser, err := s.userRepo.GetOrCreateByTelegramUserID(userID, nil)
	if err != nil {
		s.logger.Error("Failed to get or create request user", zap.Error(err))
		_, err := b.SendMessage(update.EffectiveChat.Id,
			"An error occurred. Please try again later.", nil)
		return err
	}

	// Create ban request
	blacklist, err := s.blacklistService.CreateBanRequest(s.botID, guestUserID, requestUser.ID)
	if err != nil {
		s.logger.Error("Failed to create ban request", zap.Error(err))
		_, err := b.SendMessage(update.EffectiveChat.Id,
			"Failed to create ban request. Please try again later.", nil)
		return err
	}

	// Get bot manager
	bot, err := s.botRepo.GetByID(s.botID)
	if err != nil {
		s.logger.Error("Failed to get bot", zap.Error(err))
		return err
	}

	manager, err := s.userRepo.GetByID(bot.ManagerID)
	if err != nil {
		s.logger.Error("Failed to get manager", zap.Error(err))
		return err
	}

	// Send approval request to manager
	message := fmt.Sprintf(
		"*Ban Request*\n\n"+
			"Guest User ID: %d\n"+
			"Requested by: %d\n"+
			"Chat: %d",
		guestUserID, userID, chatID,
	)

	buttons := [][]gotgbot.InlineKeyboardButton{
		{
			{
				Text:         "Approve",
				CallbackData: fmt.Sprintf("blacklist:approve:%s", blacklist.ID.String()),
			},
			{
				Text:         "Reject",
				CallbackData: fmt.Sprintf("blacklist:reject:%s", blacklist.ID.String()),
			},
		},
	}

	keyboard := gotgbot.InlineKeyboardMarkup{InlineKeyboard: buttons}
	_, err = b.SendMessage(manager.TelegramUserID, message, &gotgbot.SendMessageOpts{
		ParseMode:   "Markdown",
		ReplyMarkup: keyboard,
	})

	if err != nil {
		s.logger.Warn("Failed to send approval request to manager", zap.Error(err))
	}

	_, err = b.SendMessage(update.EffectiveChat.Id,
		"Ban request has been sent to the manager for approval.", nil)
	return err
}

func (s *Service) handleUnban(ctx context.Context, b *gotgbot.Bot, update *ext.Context) error {
	if update.EffectiveMessage.ReplyToMessage == nil {
		_, err := b.SendMessage(update.EffectiveChat.Id,
			"Please reply to a message from the user you want to unban.", nil)
		return err
	}

	replyTo := update.EffectiveMessage.ReplyToMessage
	guestUserID := replyTo.From.Id

	// Check if user has permission
	chatID := update.EffectiveChat.Id
	userID := update.EffectiveUser.Id

	// Check if chat is a recipient
	recipient, err := s.recipientRepo.GetByBotIDAndChatID(s.botID, chatID)
	if err != nil {
		_, err := b.SendMessage(update.EffectiveChat.Id,
			"This command can only be used in recipient chats.", nil)
		return err
	}

	// Check permission: Manager, Admin, or any user in recipient chat
	isManagerOrAdmin, err := s.IsManagerOrAdmin(userID)
	if err != nil {
		s.logger.Warn("Failed to check permission", zap.Error(err))
	}
	if !isManagerOrAdmin && recipient.RecipientType != models.RecipientTypeGroup {
		_, err := b.SendMessage(update.EffectiveChat.Id,
			"You are not authorized to use this command.", nil)
		return err
	}

	// Get or create request user
	requestUser, err := s.userRepo.GetOrCreateByTelegramUserID(userID, nil)
	if err != nil {
		s.logger.Error("Failed to get or create request user", zap.Error(err))
		_, err := b.SendMessage(update.EffectiveChat.Id,
			"An error occurred. Please try again later.", nil)
		return err
	}

	// Create unban request
	blacklist, err := s.blacklistService.CreateUnbanRequest(s.botID, guestUserID, requestUser.ID)
	if err != nil {
		s.logger.Error("Failed to create unban request", zap.Error(err))
		_, err := b.SendMessage(update.EffectiveChat.Id,
			"Failed to create unban request. Please try again later.", nil)
		return err
	}

	// Get bot manager
	bot, err := s.botRepo.GetByID(s.botID)
	if err != nil {
		s.logger.Error("Failed to get bot", zap.Error(err))
		return err
	}

	manager, err := s.userRepo.GetByID(bot.ManagerID)
	if err != nil {
		s.logger.Error("Failed to get manager", zap.Error(err))
		return err
	}

	// Send approval request to manager
	message := fmt.Sprintf(
		"*Unban Request*\n\n"+
			"Guest User ID: %d\n"+
			"Requested by: %d\n"+
			"Chat: %d",
		guestUserID, userID, chatID,
	)

	buttons := [][]gotgbot.InlineKeyboardButton{
		{
			{
				Text:         "Approve",
				CallbackData: fmt.Sprintf("blacklist:approve:%s", blacklist.ID.String()),
			},
			{
				Text:         "Reject",
				CallbackData: fmt.Sprintf("blacklist:reject:%s", blacklist.ID.String()),
			},
		},
	}

	keyboard := gotgbot.InlineKeyboardMarkup{InlineKeyboard: buttons}
	_, err = b.SendMessage(manager.TelegramUserID, message, &gotgbot.SendMessageOpts{
		ParseMode:   "Markdown",
		ReplyMarkup: keyboard,
	})

	if err != nil {
		s.logger.Warn("Failed to send approval request to manager", zap.Error(err))
	}

	_, err = b.SendMessage(update.EffectiveChat.Id,
		"Unban request has been sent to the manager for approval.", nil)
	return err
}

func (s *Service) handleBlacklistCallback(ctx context.Context, b *gotgbot.Bot, update *ext.Context, parts []string) error {
	if len(parts) < 2 {
		_, err := b.AnswerCallbackQuery(update.CallbackQuery.Id, &gotgbot.AnswerCallbackQueryOpts{
			Text: "Invalid callback data",
		})
		return err
	}

	action := parts[0]
	blacklistIDStr := parts[1]

	blacklistID, err := uuid.Parse(blacklistIDStr)
	if err != nil {
		_, err := b.AnswerCallbackQuery(update.CallbackQuery.Id, &gotgbot.AnswerCallbackQueryOpts{
			Text: "Invalid blacklist ID",
		})
		return err
	}

	blacklist, err := s.blacklistRepo.GetByID(blacklistID)
	if err != nil {
		_, err := b.AnswerCallbackQuery(update.CallbackQuery.Id, &gotgbot.AnswerCallbackQueryOpts{
			Text: "Blacklist request not found",
		})
		return err
	}

	// Check if user is manager
	userID := update.EffectiveUser.Id
	isManager, err := s.IsManager(userID)
	if err != nil || !isManager {
		_, err := b.AnswerCallbackQuery(update.CallbackQuery.Id, &gotgbot.AnswerCallbackQueryOpts{
			Text: "Only the manager can approve/reject requests",
		})
		return err
	}

	// Answer callback query first
	_, err = b.AnswerCallbackQuery(update.CallbackQuery.Id, &gotgbot.AnswerCallbackQueryOpts{})
	if err != nil {
		s.logger.Warn("Failed to answer callback query", zap.Error(err))
	}

	switch action {
	case "approve":
		if err := s.blacklistService.ApproveRequest(blacklistID); err != nil {
			s.logger.Error("Failed to approve request", zap.Error(err))
			_, err := b.AnswerCallbackQuery(update.CallbackQuery.Id, &gotgbot.AnswerCallbackQueryOpts{
				Text: "Failed to approve request",
			})
			return err
		}

		// Notify guest
		guest, err := s.guestRepo.GetByID(blacklist.GuestID)
		if err == nil {
			if blacklist.RequestType == models.BlacklistRequestTypeBan {
				_, _ = b.SendMessage(guest.GuestUserID,
					"You have been banned from this bot.", nil)
			} else if blacklist.RequestType == models.BlacklistRequestTypeUnban {
				_, _ = b.SendMessage(guest.GuestUserID,
					"You have been unbanned from this bot.", nil)
			}
		}

		// Log audit
		user, _ := s.userRepo.GetByTelegramUserID(userID)
		if user != nil {
			details, _ := json.Marshal(map[string]interface{}{
				"blacklist_id": blacklistID.String(),
				"request_type": blacklist.RequestType,
			})
			auditLog := &models.AuditLog{
				UserID:       &user.ID,
				ActionType:   models.AuditLogActionBan,
				ResourceType: "blacklist",
				ResourceID:   blacklistID,
				Details:      string(details),
			}
			if blacklist.RequestType == models.BlacklistRequestTypeUnban {
				auditLog.ActionType = models.AuditLogActionUnban
			}
			s.auditLogRepo.Create(auditLog)
		}

		// Note: AnswerCallbackQuery was already called at the beginning
		return nil

	case "reject":
		if err := s.blacklistService.RejectRequest(blacklistID); err != nil {
			s.logger.Error("Failed to reject request", zap.Error(err))
			_, err := b.AnswerCallbackQuery(update.CallbackQuery.Id, &gotgbot.AnswerCallbackQueryOpts{
				Text: "Failed to reject request",
			})
			return err
		}

		// Note: AnswerCallbackQuery was already called at the beginning
		return nil

	default:
		_, err := b.AnswerCallbackQuery(update.CallbackQuery.Id, &gotgbot.AnswerCallbackQueryOpts{
			Text: "Unknown action",
		})
		return err
	}
}
