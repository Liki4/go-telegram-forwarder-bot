package forwarder_bot

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"go-telegram-forwarder-bot/internal/models"
	"go-telegram-forwarder-bot/internal/utils"
	"go.uber.org/zap"
)

func (s *Service) handleAddRecipient(ctx context.Context, b *gotgbot.Bot, update *ext.Context) error {
	parts := strings.Fields(update.EffectiveMessage.Text)
	if len(parts) < 2 {
		_, err := b.SendMessage(update.EffectiveChat.Id,
			"Usage: /addrecipient <chat_id>\nExample: /addrecipient 123456789", nil)
		return err
	}

	chatID, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		_, err := b.SendMessage(update.EffectiveChat.Id,
			fmt.Sprintf("Invalid chat ID: %v", err), nil)
		return err
	}

	// Check if already exists
	existing, err := s.recipientRepo.GetByBotIDAndChatID(s.botID, chatID)
	if err == nil && existing != nil {
		_, err := b.SendMessage(update.EffectiveChat.Id,
			"This recipient is already added.", nil)
		return err
	}

	// Determine recipient type (simplified: assume user if chat_id > 0, group if < 0)
	recipientType := models.RecipientTypeUser
	if chatID < 0 {
		recipientType = models.RecipientTypeGroup
	}

	recipient := &models.Recipient{
		BotID:         s.botID,
		RecipientType: recipientType,
		ChatID:        chatID,
	}

	if err := s.recipientRepo.Create(recipient); err != nil {
		s.logger.Error("Failed to create recipient", zap.Error(err))
		_, err := b.SendMessage(update.EffectiveChat.Id,
			"Failed to add recipient. Please try again later.", nil)
		return err
	}

	// Log audit
	userID := update.EffectiveUser.Id
	user, _ := s.userRepo.GetByTelegramUserID(userID)
	if user != nil {
		details, _ := json.Marshal(map[string]interface{}{
			"chat_id": chatID,
			"type":    recipientType,
		})
		auditLog := &models.AuditLog{
			UserID:       &user.ID,
			ActionType:   models.AuditLogActionAddRecipient,
			ResourceType: "recipient",
			ResourceID:   recipient.ID,
			Details:      string(details),
		}
		s.auditLogRepo.Create(auditLog)
	}

	_, err = b.SendMessage(update.EffectiveChat.Id,
		fmt.Sprintf("Recipient %d has been added successfully!", chatID), nil)
	return err
}

func (s *Service) handleDelRecipient(ctx context.Context, b *gotgbot.Bot, update *ext.Context) error {
	parts := strings.Fields(update.EffectiveMessage.Text)
	if len(parts) < 2 {
		_, err := b.SendMessage(update.EffectiveChat.Id,
			"Usage: /delrecipient <chat_id>\nExample: /delrecipient 123456789", nil)
		return err
	}

	chatID, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		_, err := b.SendMessage(update.EffectiveChat.Id,
			fmt.Sprintf("Invalid chat ID: %v", err), nil)
		return err
	}

	recipient, err := s.recipientRepo.GetByBotIDAndChatID(s.botID, chatID)
	if err != nil {
		_, err := b.SendMessage(update.EffectiveChat.Id,
			"Recipient not found.", nil)
		return err
	}

	if err := s.recipientRepo.Delete(recipient.ID); err != nil {
		s.logger.Error("Failed to delete recipient", zap.Error(err))
		_, err := b.SendMessage(update.EffectiveChat.Id,
			"Failed to delete recipient. Please try again later.", nil)
		return err
	}

	// Log audit
	userID := update.EffectiveUser.Id
	user, _ := s.userRepo.GetByTelegramUserID(userID)
	if user != nil {
		details, _ := json.Marshal(map[string]interface{}{
			"chat_id": chatID,
		})
		auditLog := &models.AuditLog{
			UserID:       &user.ID,
			ActionType:   models.AuditLogActionDelRecipient,
			ResourceType: "recipient",
			ResourceID:   recipient.ID,
			Details:      string(details),
		}
		s.auditLogRepo.Create(auditLog)
	}

	_, err = b.SendMessage(update.EffectiveChat.Id,
		fmt.Sprintf("Recipient %d has been removed successfully!", chatID), nil)
	return err
}

func (s *Service) handleListRecipient(ctx context.Context, b *gotgbot.Bot, update *ext.Context) error {
	recipients, err := s.recipientRepo.GetByBotID(s.botID)
	if err != nil {
		s.logger.Error("Failed to get recipients", zap.Error(err))
		_, err := b.SendMessage(update.EffectiveChat.Id,
			"An error occurred. Please try again later.", nil)
		return err
	}

	if len(recipients) == 0 {
		_, err := b.SendMessage(update.EffectiveChat.Id,
			"No recipients configured.", nil)
		return err
	}

	var message strings.Builder
	message.WriteString("*Recipients:*\n\n")
	for i, recipient := range recipients {
		message.WriteString(fmt.Sprintf("%d. %s: %d\n", i+1, recipient.RecipientType, recipient.ChatID))
	}

	_, err = b.SendMessage(update.EffectiveChat.Id, message.String(), &gotgbot.SendMessageOpts{
		ParseMode: "Markdown",
	})
	return err
}

func (s *Service) handleAddAdmin(ctx context.Context, b *gotgbot.Bot, update *ext.Context) error {
	parts := strings.Fields(update.EffectiveMessage.Text)
	if len(parts) < 2 {
		_, err := b.SendMessage(update.EffectiveChat.Id,
			"Usage: /addadmin <user_id>\nExample: /addadmin 123456789", nil)
		return err
	}

	adminUserID, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		_, err := b.SendMessage(update.EffectiveChat.Id,
			fmt.Sprintf("Invalid user ID: %v", err), nil)
		return err
	}

	adminUser, err := s.userRepo.GetOrCreateByTelegramUserID(adminUserID, nil)
	if err != nil {
		s.logger.Error("Failed to get or create admin user", zap.Error(err))
		_, err := b.SendMessage(update.EffectiveChat.Id,
			"An error occurred. Please try again later.", nil)
		return err
	}

	// Check if already admin
	isAdmin, err := s.botAdminRepo.IsAdmin(s.botID, adminUser.ID)
	if err != nil {
		s.logger.Error("Failed to check admin status", zap.Error(err))
		_, err := b.SendMessage(update.EffectiveChat.Id,
			"An error occurred. Please try again later.", nil)
		return err
	}
	if isAdmin {
		_, err := b.SendMessage(update.EffectiveChat.Id,
			"This user is already an admin.", nil)
		return err
	}

	botAdmin := &models.BotAdmin{
		BotID:       s.botID,
		AdminUserID: adminUser.ID,
	}

	if err := s.botAdminRepo.Create(botAdmin); err != nil {
		s.logger.Error("Failed to create admin", zap.Error(err))
		_, err := b.SendMessage(update.EffectiveChat.Id,
			"Failed to add admin. Please try again later.", nil)
		return err
	}

	// Log audit
	userID := update.EffectiveUser.Id
	user, _ := s.userRepo.GetByTelegramUserID(userID)
	if user != nil {
		details, _ := json.Marshal(map[string]interface{}{
			"admin_user_id": adminUserID,
		})
		auditLog := &models.AuditLog{
			UserID:       &user.ID,
			ActionType:   models.AuditLogActionAddAdmin,
			ResourceType: "admin",
			ResourceID:   botAdmin.ID,
			Details:      string(details),
		}
		s.auditLogRepo.Create(auditLog)
	}

	_, err = b.SendMessage(update.EffectiveChat.Id,
		fmt.Sprintf("User %d has been added as admin successfully!", adminUserID), nil)
	return err
}

func (s *Service) handleDelAdmin(ctx context.Context, b *gotgbot.Bot, update *ext.Context) error {
	parts := strings.Fields(update.EffectiveMessage.Text)
	if len(parts) < 2 {
		_, err := b.SendMessage(update.EffectiveChat.Id,
			"Usage: /deladmin <user_id>\nExample: /deladmin 123456789", nil)
		return err
	}

	adminUserID, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		_, err := b.SendMessage(update.EffectiveChat.Id,
			fmt.Sprintf("Invalid user ID: %v", err), nil)
		return err
	}

	adminUser, err := s.userRepo.GetByTelegramUserID(adminUserID)
	if err != nil {
		_, err := b.SendMessage(update.EffectiveChat.Id,
			"User not found.", nil)
		return err
	}

	botAdmin, err := s.botAdminRepo.GetByBotIDAndUserID(s.botID, adminUser.ID)
	if err != nil {
		_, err := b.SendMessage(update.EffectiveChat.Id,
			"This user is not an admin.", nil)
		return err
	}

	if err := s.botAdminRepo.Delete(botAdmin.ID); err != nil {
		s.logger.Error("Failed to delete admin", zap.Error(err))
		_, err := b.SendMessage(update.EffectiveChat.Id,
			"Failed to remove admin. Please try again later.", nil)
		return err
	}

	// Log audit
	userID := update.EffectiveUser.Id
	user, _ := s.userRepo.GetByTelegramUserID(userID)
	if user != nil {
		details, _ := json.Marshal(map[string]interface{}{
			"admin_user_id": adminUserID,
		})
		auditLog := &models.AuditLog{
			UserID:       &user.ID,
			ActionType:   models.AuditLogActionDelAdmin,
			ResourceType: "admin",
			ResourceID:   botAdmin.ID,
			Details:      string(details),
		}
		s.auditLogRepo.Create(auditLog)
	}

	_, err = b.SendMessage(update.EffectiveChat.Id,
		fmt.Sprintf("User %d has been removed from admins successfully!", adminUserID), nil)
	return err
}

func (s *Service) handleListAdmins(ctx context.Context, b *gotgbot.Bot, update *ext.Context) error {
	admins, err := s.botAdminRepo.GetByBotID(s.botID)
	if err != nil {
		s.logger.Error("Failed to get admins", zap.Error(err))
		_, err := b.SendMessage(update.EffectiveChat.Id,
			"An error occurred. Please try again later.", nil)
		return err
	}

	if len(admins) == 0 {
		_, err := b.SendMessage(update.EffectiveChat.Id,
			"No admins configured.", nil)
		return err
	}

	var message strings.Builder
	message.WriteString("*Admins:*\n\n")
	for i, admin := range admins {
		username := "Unknown"
		if admin.AdminUser.Username != nil {
			username = *admin.AdminUser.Username
		}
		message.WriteString(fmt.Sprintf("%d. @%s (%d)\n", i+1, utils.EscapeMarkdown(username), admin.AdminUser.TelegramUserID))
	}

	_, err = b.SendMessage(update.EffectiveChat.Id, message.String(), &gotgbot.SendMessageOpts{
		ParseMode: "Markdown",
	})
	return err
}

func (s *Service) handleStats(ctx context.Context, b *gotgbot.Bot, update *ext.Context) error {
	stats, err := s.statsService.GetBotStatistics(s.botID)
	if err != nil {
		s.logger.Error("Failed to get statistics", zap.Error(err))
		_, err := b.SendMessage(update.EffectiveChat.Id,
			"Failed to retrieve statistics. Please try again later.", nil)
		return err
	}

	message := fmt.Sprintf(
		"*Bot Statistics*\n\n"+
			"Inbound Messages: %d\n"+
			"Outbound Messages: %d\n"+
			"Total Guests: %d",
		stats.InboundCount,
		stats.OutboundCount,
		stats.GuestCount,
	)

	_, err = b.SendMessage(update.EffectiveChat.Id, message, &gotgbot.SendMessageOpts{
		ParseMode: "Markdown",
	})
	return err
}

func (s *Service) handleHelp(ctx context.Context, b *gotgbot.Bot, update *ext.Context) error {
	userID := update.EffectiveUser.Id
	chatID := update.EffectiveChat.Id
	isManager, _ := s.IsManager(userID)
	isManagerOrAdmin, _ := s.IsManagerOrAdmin(userID)

	// Check if user is a recipient
	isRecipient := false
	_, err := s.recipientRepo.GetByBotIDAndChatID(s.botID, chatID)
	if err == nil {
		isRecipient = true
	}

	// Determine if user is a pure guest (not manager, not admin, not recipient)
	isPureGuest := !isManagerOrAdmin && !isRecipient

	helpText := "*ForwarderBot Commands*\n\n"
	helpText += "*/help* - Show this help message\n"

	if isManagerOrAdmin {
		helpText += "\n*Recipient Management:*\n"
		helpText += "*/addrecipient <chat_id>* - Add a recipient\n"
		helpText += "*/delrecipient <chat_id>* - Remove a recipient\n"
		helpText += "*/listrecipient* - List all recipients\n"
	}

	if isManagerOrAdmin {
		helpText += "\n*Admin Management:*\n"
		if isManager {
			helpText += "*/addadmin <user_id>* - Add an admin (Manager only)\n"
			helpText += "*/deladmin <user_id>* - Remove an admin (Manager only)\n"
		}
		helpText += "*/listadmins* - List all admins\n"
	}

	if isManagerOrAdmin {
		helpText += "\n*Statistics:*\n"
		helpText += "*/stats* - View bot statistics\n"
	}

	helpText += "\n*Blacklist Management:*\n"
	// Only show /ban command if user is not a pure guest
	if !isPureGuest {
		helpText += "*/ban* - Ban a guest (reply to their message)\n"
	}
	helpText += "*/unban* - Unban a guest (reply to their message, or use directly to request unban for yourself)\n"
	
	if !isPureGuest {
		helpText += "\n*Note:*\n"
		helpText += "- Ban command can be used by Manager, Admins, or any user in a group recipient\n"
		helpText += "- Unban command: Reply to a message to unban someone else (requires permission), or use directly to request unban for yourself if you are blacklisted"
	} else {
		helpText += "\n*Note:*\n"
		helpText += "- Unban command: Use directly to request unban for yourself if you are blacklisted"
	}

	helpText += "\n\n*How it works:*\n"
	helpText += "1. Guests send messages to this bot\n"
	helpText += "2. Messages are forwarded to all recipients\n"
	helpText += "3. Recipients can reply to forward messages back to guests"

	_, err = b.SendMessage(update.EffectiveChat.Id, helpText, &gotgbot.SendMessageOpts{
		ParseMode: "Markdown",
	})
	return err
}
