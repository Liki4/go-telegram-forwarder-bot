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

// sendApprovalRequestToManagersAndAdmins sends approval request to manager and all admins
// and stores the message IDs for later editing
func (s *Service) sendApprovalRequestToManagersAndAdmins(
	ctx context.Context,
	b *gotgbot.Bot,
	blacklistID uuid.UUID,
	messageText string,
	buttons [][]gotgbot.InlineKeyboardButton,
) error {
	// Get bot manager
	bot, err := s.botRepo.GetByID(s.botID)
	if err != nil {
		return fmt.Errorf("failed to get bot: %w", err)
	}

	manager, err := s.userRepo.GetByID(bot.ManagerID)
	if err != nil {
		return fmt.Errorf("failed to get manager: %w", err)
	}

	// Get all admins
	admins, err := s.botAdminRepo.GetByBotID(s.botID)
	if err != nil {
		s.logger.Warn("Failed to get admins", zap.Error(err))
		admins = []*models.BotAdmin{}
	}

	keyboard := gotgbot.InlineKeyboardMarkup{InlineKeyboard: buttons}

	// Send to manager
	managerMsg, err := b.SendMessage(manager.TelegramUserID, messageText, &gotgbot.SendMessageOpts{
		ParseMode:   "Markdown",
		ReplyMarkup: keyboard,
	})
	if err != nil {
		s.logger.Warn("Failed to send approval request to manager", zap.Error(err))
	} else {
		// Store message ID
		approvalMsg := &models.BlacklistApprovalMessage{
			BlacklistID: blacklistID,
			UserID:      manager.ID,
			ChatID:      manager.TelegramUserID,
			MessageID:   managerMsg.MessageId,
		}
		if err := s.blacklistApprovalMessageRepo.Create(approvalMsg); err != nil {
			s.logger.Warn("Failed to store approval message for manager", zap.Error(err))
		}
	}

	// Send to all admins
	for _, admin := range admins {
		adminMsg, err := b.SendMessage(admin.AdminUser.TelegramUserID, messageText, &gotgbot.SendMessageOpts{
			ParseMode:   "Markdown",
			ReplyMarkup: keyboard,
		})
		if err != nil {
			s.logger.Warn("Failed to send approval request to admin",
				zap.String("admin_id", admin.AdminUser.ID.String()),
				zap.Error(err))
			continue
		}

		// Store message ID
		approvalMsg := &models.BlacklistApprovalMessage{
			BlacklistID: blacklistID,
			UserID:      admin.AdminUser.ID,
			ChatID:      admin.AdminUser.TelegramUserID,
			MessageID:   adminMsg.MessageId,
		}
		if err := s.blacklistApprovalMessageRepo.Create(approvalMsg); err != nil {
			s.logger.Warn("Failed to store approval message for admin",
				zap.String("admin_id", admin.AdminUser.ID.String()),
				zap.Error(err))
		}
	}

	return nil
}

func (s *Service) handleBan(ctx context.Context, b *gotgbot.Bot, update *ext.Context) error {
	if update.EffectiveMessage.ReplyToMessage == nil {
		_, err := b.SendMessage(update.EffectiveChat.Id,
			"Please reply to a message from the user you want to ban.", nil)
		return err
	}

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

	// Get guest user ID from message mapping
	// The replied message is in recipient chat, so we need to find the corresponding guest
	replyTo := update.EffectiveMessage.ReplyToMessage
	recipientMessageID := replyTo.MessageId

	s.logger.Debug("Finding guest user ID from message mapping",
		zap.String("bot_id", s.botID.String()),
		zap.Int64("recipient_chat_id", chatID),
		zap.Int64("recipient_message_id", recipientMessageID))

	mapping, err := s.messageMappingRepo.GetByRecipientMessage(s.botID, chatID, recipientMessageID)
	if err != nil {
		s.logger.Debug("Failed to find message mapping for ban",
			zap.String("bot_id", s.botID.String()),
			zap.Int64("recipient_chat_id", chatID),
			zap.Int64("recipient_message_id", recipientMessageID),
			zap.Error(err))
		_, err := b.SendMessage(update.EffectiveChat.Id,
			"Failed to find the corresponding guest. Please make sure you are replying to a forwarded message.", nil)
		return err
	}

	// For private chats, GuestChatID equals GuestUserID
	// For group chats, we would need to query the guest table, but guests are always private chats
	guestUserID := mapping.GuestChatID

	s.logger.Debug("Found guest user ID from message mapping",
		zap.String("bot_id", s.botID.String()),
		zap.Int64("guest_user_id", guestUserID),
		zap.Int64("guest_chat_id", mapping.GuestChatID),
		zap.Int64("guest_message_id", mapping.GuestMessageID))

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

	// Send approval request to manager and all admins
	message := fmt.Sprintf(
		"*Ban Request*\n\n"+
			"Guest User ID: `%d`\n"+
			"Requested by: `%d`\n"+
			"Chat: `%d`",
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

	if err := s.sendApprovalRequestToManagersAndAdmins(ctx, b, blacklist.ID, message, buttons); err != nil {
		s.logger.Warn("Failed to send approval request", zap.Error(err))
	}

	_, err = b.SendMessage(update.EffectiveChat.Id,
		"Ban request has been sent to the manager for approval.", nil)
	return err
}

func (s *Service) handleUnban(ctx context.Context, b *gotgbot.Bot, update *ext.Context) error {
	userID := update.EffectiveUser.Id
	chatID := update.EffectiveChat.Id
	var guestUserID int64
	var isSelfRequest bool

	// Check if this is a reply (admin/manager unbanning someone else) or self-request
	if update.EffectiveMessage.ReplyToMessage == nil {
		// Self-request: user wants to unban themselves
		isSelfRequest = true
		guestUserID = userID

		// Check if user is actually blacklisted
		isBlacklisted, err := s.blacklistService.IsBlacklisted(s.botID, guestUserID)
		if err != nil {
			s.logger.Warn("Failed to check blacklist status", zap.Error(err))
			_, err := b.SendMessage(update.EffectiveChat.Id,
				"An error occurred while checking your status. Please try again later.", nil)
			return err
		}

		if !isBlacklisted {
			_, err := b.SendMessage(update.EffectiveChat.Id,
				"You are not currently blacklisted.", nil)
			return err
		}
	} else {
		// Reply mode: admin/manager unbanning someone else
		isSelfRequest = false

		// Check if chat is a recipient
		recipient, err := s.recipientRepo.GetByBotIDAndChatID(s.botID, chatID)
		if err != nil {
			_, err := b.SendMessage(update.EffectiveChat.Id,
				"This command can only be used in recipient chats.", nil)
			return err
		}

		// Get guest user ID from message mapping
		// The replied message is in recipient chat, so we need to find the corresponding guest
		replyTo := update.EffectiveMessage.ReplyToMessage
		recipientMessageID := replyTo.MessageId

		s.logger.Debug("Finding guest user ID from message mapping for unban",
			zap.String("bot_id", s.botID.String()),
			zap.Int64("recipient_chat_id", chatID),
			zap.Int64("recipient_message_id", recipientMessageID))

		mapping, err := s.messageMappingRepo.GetByRecipientMessage(s.botID, chatID, recipientMessageID)
		if err != nil {
			s.logger.Debug("Failed to find message mapping for unban",
				zap.String("bot_id", s.botID.String()),
				zap.Int64("recipient_chat_id", chatID),
				zap.Int64("recipient_message_id", recipientMessageID),
				zap.Error(err))
			_, err := b.SendMessage(update.EffectiveChat.Id,
				"Failed to find the corresponding guest. Please make sure you are replying to a forwarded message.", nil)
			return err
		}

		// For private chats, GuestChatID equals GuestUserID
		// For group chats, we would need to query the guest table, but guests are always private chats
		guestUserID = mapping.GuestChatID

		s.logger.Debug("Found guest user ID from message mapping for unban",
			zap.String("bot_id", s.botID.String()),
			zap.Int64("guest_user_id", guestUserID),
			zap.Int64("guest_chat_id", mapping.GuestChatID),
			zap.Int64("guest_message_id", mapping.GuestMessageID))

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

	// Send approval request to manager and all admins
	var message string
	if isSelfRequest {
		message = fmt.Sprintf(
			"*Unban Request (Self-Request)*\n\n"+
				"Guest User ID: `%d`\n"+
				"Requested by: `%d`\n"+
				"*Note:* This is a self-request to remove blacklist status.",
			guestUserID, userID,
		)
	} else {
		message = fmt.Sprintf(
			"*Unban Request*\n\n"+
				"Guest User ID: `%d`\n"+
				"Requested by: `%d`\n"+
				"Chat: `%d`",
			guestUserID, userID, chatID,
		)
	}

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

	if err := s.sendApprovalRequestToManagersAndAdmins(ctx, b, blacklist.ID, message, buttons); err != nil {
		s.logger.Warn("Failed to send approval request", zap.Error(err))
	}

	var responseMessage string
	if isSelfRequest {
		responseMessage = "Your unban request has been sent to the manager for approval. It will be automatically approved after 24 hours if not manually reviewed."
	} else {
		responseMessage = "Unban request has been sent to the manager for approval."
	}

	_, err = b.SendMessage(update.EffectiveChat.Id, responseMessage, nil)
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

	// Check if user is manager or admin
	userID := update.EffectiveUser.Id
	isManagerOrAdmin, err := s.IsManagerOrAdmin(userID)
	if err != nil || !isManagerOrAdmin {
		_, err := b.AnswerCallbackQuery(update.CallbackQuery.Id, &gotgbot.AnswerCallbackQueryOpts{
			Text: "Only the manager or admin can approve/reject requests",
		})
		return err
	}

	// Get or create user to get user ID
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

	// Get executor's display name
	executorName := fmt.Sprintf("%d", userID)
	if user.Username != nil && *user.Username != "" {
		executorName = "@" + *user.Username
	}

	// Answer callback query first
	_, err = b.AnswerCallbackQuery(update.CallbackQuery.Id, &gotgbot.AnswerCallbackQueryOpts{})
	if err != nil {
		s.logger.Warn("Failed to answer callback query", zap.Error(err))
	}

	// Get all approval messages for this blacklist request
	approvalMessages, err := s.blacklistApprovalMessageRepo.GetByBlacklistID(blacklistID)
	if err != nil {
		s.logger.Warn("Failed to get approval messages", zap.Error(err))
		approvalMessages = []*models.BlacklistApprovalMessage{}
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

		// Edit all approval messages
		s.editApprovalMessages(ctx, b, blacklist, approvalMessages, user.ID, executorName, "approved")

		return nil

	case "reject":
		if err := s.blacklistService.RejectRequest(blacklistID); err != nil {
			s.logger.Error("Failed to reject request", zap.Error(err))
			_, err := b.AnswerCallbackQuery(update.CallbackQuery.Id, &gotgbot.AnswerCallbackQueryOpts{
				Text: "Failed to reject request",
			})
			return err
		}

		// Edit all approval messages
		s.editApprovalMessages(ctx, b, blacklist, approvalMessages, user.ID, executorName, "rejected")

		return nil

	default:
		_, err := b.AnswerCallbackQuery(update.CallbackQuery.Id, &gotgbot.AnswerCallbackQueryOpts{
			Text: "Unknown action",
		})
		return err
	}
}

// editApprovalMessages edits all approval messages to show the result
func (s *Service) editApprovalMessages(
	ctx context.Context,
	b *gotgbot.Bot,
	blacklist *models.Blacklist,
	approvalMessages []*models.BlacklistApprovalMessage,
	executorUserID uuid.UUID,
	executorName string,
	status string, // "approved" or "rejected"
) {
	// Build the message text based on request type
	var requestTypeText string
	if blacklist.RequestType == models.BlacklistRequestTypeBan {
		requestTypeText = "Ban Request"
	} else {
		requestTypeText = "Unban Request"
	}

	// Get guest info for message
	guest, err := s.guestRepo.GetByID(blacklist.GuestID)
	var guestUserID int64
	if err == nil {
		guestUserID = guest.GuestUserID
	}

	// Get request user info
	requestUser, _ := s.userRepo.GetByID(blacklist.RequestUserID)
	var requestUserID int64
	if requestUser != nil {
		requestUserID = requestUser.TelegramUserID
	}

	// Build base message text
	baseMessage := fmt.Sprintf(
		"*%s*\n\n"+
			"Guest User ID: `%d`\n"+
			"Requested by: `%d`\n",
		requestTypeText, guestUserID, requestUserID)

	// Edit each message
	for _, msg := range approvalMessages {
		var buttonText string
		var messageText string

		if msg.UserID == executorUserID {
			// Executor's message: show status only
			if status == "approved" {
				buttonText = "Approved"
				messageText = baseMessage + "\n*Status: Approved*"
			} else {
				buttonText = "Rejected"
				messageText = baseMessage + "\n*Status: Rejected*"
			}
		} else {
			// Other users' messages: show who did it
			if status == "approved" {
				buttonText = fmt.Sprintf("Approved by %s", executorName)
				messageText = baseMessage + fmt.Sprintf("\n*Status: Approved by %s*", executorName)
			} else {
				buttonText = fmt.Sprintf("Rejected by %s", executorName)
				messageText = baseMessage + fmt.Sprintf("\n*Status: Rejected by %s*", executorName)
			}
		}

		// Create button with status
		buttons := [][]gotgbot.InlineKeyboardButton{
			{
				{
					Text:         buttonText,
					CallbackData: "blacklist:status:" + blacklist.ID.String(), // Non-functional button
				},
			},
		}
		keyboard := gotgbot.InlineKeyboardMarkup{InlineKeyboard: buttons}

		// Edit the message
		_, _, err := b.EditMessageText(messageText, &gotgbot.EditMessageTextOpts{
			ChatId:      msg.ChatID,
			MessageId:   msg.MessageID,
			ParseMode:   "Markdown",
			ReplyMarkup: keyboard,
		})
		if err != nil {
			s.logger.Warn("Failed to edit approval message",
				zap.String("blacklist_id", blacklist.ID.String()),
				zap.String("user_id", msg.UserID.String()),
				zap.Int64("chat_id", msg.ChatID),
				zap.Int64("message_id", msg.MessageID),
				zap.Error(err))
		}
	}
}
