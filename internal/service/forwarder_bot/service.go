package forwarder_bot

import (
	"context"
	"fmt"
	"strings"

	"go-telegram-forwarder-bot/internal/config"
	"go-telegram-forwarder-bot/internal/repository"
	"go-telegram-forwarder-bot/internal/service/blacklist"
	"go-telegram-forwarder-bot/internal/service/message"
	"go-telegram-forwarder-bot/internal/service/statistics"
	"go-telegram-forwarder-bot/internal/utils"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type Service struct {
	botID              uuid.UUID
	botRepo            repository.BotRepository
	recipientRepo      repository.RecipientRepository
	guestRepo          repository.GuestRepository
	blacklistRepo      repository.BlacklistRepository
	botAdminRepo       repository.BotAdminRepository
	messageMappingRepo repository.MessageMappingRepository
	userRepo           repository.UserRepository
	auditLogRepo       repository.AuditLogRepository
	messageForwarder   *message.Forwarder
	blacklistService   *blacklist.Service
	statsService       *statistics.Service
	config             *config.Config
	logger             *zap.Logger
	encryptionKey      []byte
}

func NewService(
	botID uuid.UUID,
	botRepo repository.BotRepository,
	recipientRepo repository.RecipientRepository,
	guestRepo repository.GuestRepository,
	blacklistRepo repository.BlacklistRepository,
	botAdminRepo repository.BotAdminRepository,
	messageMappingRepo repository.MessageMappingRepository,
	userRepo repository.UserRepository,
	auditLogRepo repository.AuditLogRepository,
	messageForwarder *message.Forwarder,
	blacklistService *blacklist.Service,
	statsService *statistics.Service,
	cfg *config.Config,
	logger *zap.Logger,
) (*Service, error) {
	key, err := utils.GetEncryptionKeyFromConfig(cfg.EncryptionKey)
	if err != nil {
		return nil, fmt.Errorf("failed to get encryption key: %w", err)
	}

	return &Service{
		botID:              botID,
		botRepo:            botRepo,
		recipientRepo:      recipientRepo,
		guestRepo:          guestRepo,
		blacklistRepo:      blacklistRepo,
		botAdminRepo:       botAdminRepo,
		messageMappingRepo: messageMappingRepo,
		userRepo:           userRepo,
		auditLogRepo:       auditLogRepo,
		messageForwarder:   messageForwarder,
		blacklistService:   blacklistService,
		statsService:       statsService,
		config:             cfg,
		logger:             logger,
		encryptionKey:      key,
	}, nil
}

func (s *Service) IsManager(userID int64) (bool, error) {
	s.logger.Debug("Checking if user is manager",
		zap.String("bot_id", s.botID.String()),
		zap.Int64("user_id", userID))
	bot, err := s.botRepo.GetByID(s.botID)
	if err != nil {
		s.logger.Debug("Failed to get bot for manager check",
			zap.String("bot_id", s.botID.String()),
			zap.Int64("user_id", userID),
			zap.Error(err))
		return false, err
	}

	user, err := s.userRepo.GetByTelegramUserID(userID)
	if err != nil {
		s.logger.Debug("Failed to get user for manager check",
			zap.String("bot_id", s.botID.String()),
			zap.Int64("user_id", userID),
			zap.Error(err))
		return false, err
	}

	isManager := user.ID == bot.ManagerID
	s.logger.Debug("Manager check result",
		zap.String("bot_id", s.botID.String()),
		zap.Int64("user_id", userID),
		zap.Bool("is_manager", isManager),
		zap.String("user_uuid", user.ID.String()),
		zap.String("bot_manager_uuid", bot.ManagerID.String()))
	return isManager, nil
}

func (s *Service) IsAdmin(userID int64) (bool, error) {
	s.logger.Debug("Checking if user is admin",
		zap.String("bot_id", s.botID.String()),
		zap.Int64("user_id", userID))
	user, err := s.userRepo.GetByTelegramUserID(userID)
	if err != nil {
		s.logger.Debug("Failed to get user for admin check",
			zap.String("bot_id", s.botID.String()),
			zap.Int64("user_id", userID),
			zap.Error(err))
		return false, err
	}

	isAdmin, err := s.botAdminRepo.IsAdmin(s.botID, user.ID)
	if err != nil {
		s.logger.Debug("Failed to check admin status",
			zap.String("bot_id", s.botID.String()),
			zap.Int64("user_id", userID),
			zap.Error(err))
	} else {
		s.logger.Debug("Admin check result",
			zap.String("bot_id", s.botID.String()),
			zap.Int64("user_id", userID),
			zap.Bool("is_admin", isAdmin))
	}
	return isAdmin, err
}

func (s *Service) IsManagerOrAdmin(userID int64) (bool, error) {
	s.logger.Debug("Checking if user is manager or admin",
		zap.String("bot_id", s.botID.String()),
		zap.Int64("user_id", userID))
	isManager, err := s.IsManager(userID)
	if err != nil {
		return false, err
	}
	if isManager {
		s.logger.Debug("User is manager, skipping admin check",
			zap.String("bot_id", s.botID.String()),
			zap.Int64("user_id", userID))
		return true, nil
	}

	return s.IsAdmin(userID)
}

func (s *Service) HandleMessage(ctx context.Context, b *gotgbot.Bot, update *ext.Context) error {
	message := update.EffectiveMessage
	chatID := update.EffectiveChat.Id
	userID := update.EffectiveUser.Id
	messageID := message.MessageId

	s.logger.Debug("ForwarderBot message received",
		zap.String("bot_id", s.botID.String()),
		zap.Int64("message_id", messageID),
		zap.Int64("user_id", userID),
		zap.Int64("chat_id", chatID),
		zap.String("text", message.Text),
		zap.Bool("is_reply", message.ReplyToMessage != nil))

	// Check if message is a command
	if message.Text != "" && strings.HasPrefix(message.Text, "/") {
		s.logger.Debug("Message is a command, delegating to HandleCommand",
			zap.String("bot_id", s.botID.String()),
			zap.Int64("message_id", messageID),
			zap.String("command", message.Text))
		return s.HandleCommand(ctx, b, update)
	}

	// Check if message is a reply
	if message.ReplyToMessage != nil {
		s.logger.Debug("Message is a reply, delegating to HandleReply",
			zap.String("bot_id", s.botID.String()),
			zap.Int64("message_id", messageID),
			zap.Int64("reply_to_message_id", message.ReplyToMessage.MessageId))
		return s.HandleReply(ctx, b, update)
	}

	// Check if user is blacklisted
	s.logger.Debug("Checking if user is blacklisted",
		zap.String("bot_id", s.botID.String()),
		zap.Int64("user_id", userID))
	isBlacklisted, err := s.blacklistService.IsBlacklisted(s.botID, userID)
	if err != nil {
		s.logger.Warn("Failed to check blacklist", zap.Error(err))
	} else if isBlacklisted {
		s.logger.Debug("User is blacklisted, ignoring message",
			zap.String("bot_id", s.botID.String()),
			zap.Int64("user_id", userID),
			zap.Int64("message_id", messageID))
		return nil
	}
	s.logger.Debug("User is not blacklisted, proceeding with forwarding",
		zap.String("bot_id", s.botID.String()),
		zap.Int64("user_id", userID),
		zap.Int64("message_id", messageID))

	// Forward message to all recipients
	s.logger.Debug("Forwarding message to recipients",
		zap.String("bot_id", s.botID.String()),
		zap.Int64("message_id", messageID),
		zap.Int64("guest_chat_id", chatID))
	result, err := s.messageForwarder.ForwardToRecipients(ctx, b, s.botID, chatID, message)
	if err != nil {
		s.logger.Error("Failed to forward message", zap.Error(err))
		return err
	}

	s.logger.Debug("Message forwarding completed",
		zap.String("bot_id", s.botID.String()),
		zap.Int64("message_id", messageID),
		zap.Int("success_count", result.SuccessCount),
		zap.Int("failure_count", result.FailureCount))

	if result.FailureCount > 0 {
		s.logger.Warn("Some messages failed to forward",
			zap.String("bot_id", s.botID.String()),
			zap.Int64("message_id", messageID),
			zap.Int("success", result.SuccessCount),
			zap.Int("failures", result.FailureCount))
	}

	return nil
}

func (s *Service) HandleReply(ctx context.Context, b *gotgbot.Bot, update *ext.Context) error {
	replyMessage := update.EffectiveMessage
	chatID := update.EffectiveChat.Id
	messageID := replyMessage.MessageId
	replyToMessageID := int64(0)
	if replyMessage.ReplyToMessage != nil {
		replyToMessageID = replyMessage.ReplyToMessage.MessageId
	}

	s.logger.Debug("ForwarderBot reply received",
		zap.String("bot_id", s.botID.String()),
		zap.Int64("message_id", messageID),
		zap.Int64("reply_to_message_id", replyToMessageID),
		zap.Int64("chat_id", chatID))

	// Check if reply is from a recipient
	s.logger.Debug("Checking if reply is from a recipient",
		zap.String("bot_id", s.botID.String()),
		zap.Int64("chat_id", chatID))
	_, err := s.recipientRepo.GetByBotIDAndChatID(s.botID, chatID)
	if err != nil {
		s.logger.Debug("Reply is not from a recipient, ignoring",
			zap.String("bot_id", s.botID.String()),
			zap.Int64("chat_id", chatID),
			zap.Error(err))
		return nil
	}

	s.logger.Debug("Reply is from a recipient, forwarding to guest",
		zap.String("bot_id", s.botID.String()),
		zap.Int64("message_id", messageID),
		zap.Int64("recipient_chat_id", chatID))
	err = s.messageForwarder.ForwardReplyToGuest(ctx, b, s.botID, chatID, replyMessage)
	if err != nil {
		s.logger.Debug("Failed to forward reply to guest",
			zap.String("bot_id", s.botID.String()),
			zap.Int64("message_id", messageID),
			zap.Error(err))
	} else {
		s.logger.Debug("Reply forwarded to guest successfully",
			zap.String("bot_id", s.botID.String()),
			zap.Int64("message_id", messageID))
	}
	return err
}

func (s *Service) HandleCommand(ctx context.Context, b *gotgbot.Bot, update *ext.Context) error {
	command := update.EffectiveMessage.Text
	if command == "" {
		return nil
	}

	userID := update.EffectiveUser.Id
	chatID := update.EffectiveChat.Id

	s.logger.Debug("ForwarderBot command received",
		zap.String("bot_id", s.botID.String()),
		zap.Int64("user_id", userID),
		zap.Int64("chat_id", chatID),
		zap.String("command", command))

	switch {
	case strings.HasPrefix(command, "/help"):
		s.logger.Debug("Handling /help command",
			zap.String("bot_id", s.botID.String()),
			zap.Int64("user_id", userID))
		return s.handleHelp(ctx, b, update)
	case strings.HasPrefix(command, "/addrecipient"):
		s.logger.Debug("Handling /addrecipient command",
			zap.String("bot_id", s.botID.String()),
			zap.Int64("user_id", userID))
		isManagerOrAdmin, err := s.IsManagerOrAdmin(userID)
		if err != nil || !isManagerOrAdmin {
			s.logger.Debug("Access denied for /addrecipient",
				zap.String("bot_id", s.botID.String()),
				zap.Int64("user_id", userID),
				zap.Bool("is_manager_or_admin", isManagerOrAdmin))
			_, err := b.SendMessage(update.EffectiveChat.Id, "You are not authorized to use this command.", nil)
			return err
		}
		return s.handleAddRecipient(ctx, b, update)
	case strings.HasPrefix(command, "/delrecipient"):
		s.logger.Debug("Handling /delrecipient command",
			zap.String("bot_id", s.botID.String()),
			zap.Int64("user_id", userID))
		isManagerOrAdmin, err := s.IsManagerOrAdmin(userID)
		if err != nil || !isManagerOrAdmin {
			s.logger.Debug("Access denied for /delrecipient",
				zap.String("bot_id", s.botID.String()),
				zap.Int64("user_id", userID))
			_, err := b.SendMessage(update.EffectiveChat.Id, "You are not authorized to use this command.", nil)
			return err
		}
		return s.handleDelRecipient(ctx, b, update)
	case strings.HasPrefix(command, "/listrecipient"):
		s.logger.Debug("Handling /listrecipient command",
			zap.String("bot_id", s.botID.String()),
			zap.Int64("user_id", userID))
		isManagerOrAdmin, err := s.IsManagerOrAdmin(userID)
		if err != nil || !isManagerOrAdmin {
			s.logger.Debug("Access denied for /listrecipient",
				zap.String("bot_id", s.botID.String()),
				zap.Int64("user_id", userID))
			_, err := b.SendMessage(update.EffectiveChat.Id, "You are not authorized to use this command.", nil)
			return err
		}
		return s.handleListRecipient(ctx, b, update)
	case strings.HasPrefix(command, "/addadmin"):
		s.logger.Debug("Handling /addadmin command",
			zap.String("bot_id", s.botID.String()),
			zap.Int64("user_id", userID))
		isManager, err := s.IsManager(userID)
		if err != nil || !isManager {
			s.logger.Debug("Access denied for /addadmin - not manager",
				zap.String("bot_id", s.botID.String()),
				zap.Int64("user_id", userID))
			_, err := b.SendMessage(update.EffectiveChat.Id, "Only the manager can use this command.", nil)
			return err
		}
		return s.handleAddAdmin(ctx, b, update)
	case strings.HasPrefix(command, "/deladmin"):
		s.logger.Debug("Handling /deladmin command",
			zap.String("bot_id", s.botID.String()),
			zap.Int64("user_id", userID))
		isManager, err := s.IsManager(userID)
		if err != nil || !isManager {
			s.logger.Debug("Access denied for /deladmin - not manager",
				zap.String("bot_id", s.botID.String()),
				zap.Int64("user_id", userID))
			_, err := b.SendMessage(update.EffectiveChat.Id, "Only the manager can use this command.", nil)
			return err
		}
		return s.handleDelAdmin(ctx, b, update)
	case strings.HasPrefix(command, "/listadmins"):
		s.logger.Debug("Handling /listadmins command",
			zap.String("bot_id", s.botID.String()),
			zap.Int64("user_id", userID))
		isManagerOrAdmin, err := s.IsManagerOrAdmin(userID)
		if err != nil || !isManagerOrAdmin {
			s.logger.Debug("Access denied for /listadmins",
				zap.String("bot_id", s.botID.String()),
				zap.Int64("user_id", userID))
			_, err := b.SendMessage(update.EffectiveChat.Id, "You are not authorized to use this command.", nil)
			return err
		}
		return s.handleListAdmins(ctx, b, update)
	case strings.HasPrefix(command, "/stats"):
		s.logger.Debug("Handling /stats command",
			zap.String("bot_id", s.botID.String()),
			zap.Int64("user_id", userID))
		isManagerOrAdmin, err := s.IsManagerOrAdmin(userID)
		if err != nil || !isManagerOrAdmin {
			s.logger.Debug("Access denied for /stats",
				zap.String("bot_id", s.botID.String()),
				zap.Int64("user_id", userID))
			_, err := b.SendMessage(update.EffectiveChat.Id, "You are not authorized to use this command.", nil)
			return err
		}
		return s.handleStats(ctx, b, update)
	case strings.HasPrefix(command, "/ban"):
		s.logger.Debug("Handling /ban command",
			zap.String("bot_id", s.botID.String()),
			zap.Int64("user_id", userID))
		return s.handleBan(ctx, b, update)
	case strings.HasPrefix(command, "/unban"):
		s.logger.Debug("Handling /unban command",
			zap.String("bot_id", s.botID.String()),
			zap.Int64("user_id", userID))
		return s.handleUnban(ctx, b, update)
	default:
		s.logger.Debug("Unknown command received",
			zap.Int64("user_id", userID),
			zap.Int64("chat_id", chatID),
			zap.String("command", command))
		_, err := b.SendMessage(update.EffectiveChat.Id, "Unknown command. Use /help for available commands.", nil)
		return err
	}
}

func (s *Service) HandleCallback(ctx context.Context, b *gotgbot.Bot, update *ext.Context) error {
	userID := update.EffectiveUser.Id
	data := update.CallbackQuery.Data
	parts := strings.Split(data, ":")

	s.logger.Debug("ForwarderBot callback received",
		zap.String("bot_id", s.botID.String()),
		zap.Int64("user_id", userID),
		zap.String("callback_data", data),
		zap.Strings("parts", parts),
		zap.Int("parts_count", len(parts)))

	if len(parts) < 2 {
		s.logger.Debug("Invalid callback data format",
			zap.String("bot_id", s.botID.String()),
			zap.Int64("user_id", userID),
			zap.String("callback_data", data),
			zap.Int("parts_count", len(parts)))
		return fmt.Errorf("invalid callback data: %s", data)
	}

	action := parts[0]
	s.logger.Debug("Processing callback action",
		zap.String("bot_id", s.botID.String()),
		zap.Int64("user_id", userID),
		zap.String("action", action))

	var err error
	switch action {
	case "blacklist":
		s.logger.Debug("Handling blacklist callback",
			zap.String("bot_id", s.botID.String()),
			zap.Int64("user_id", userID),
			zap.Strings("sub_parts", parts[1:]))
		err = s.handleBlacklistCallback(ctx, b, update, parts[1:])
	default:
		s.logger.Debug("Unknown callback action",
			zap.String("bot_id", s.botID.String()),
			zap.Int64("user_id", userID),
			zap.String("action", action))
		err = fmt.Errorf("unknown callback action: %s", action)
	}

	if err != nil {
		s.logger.Debug("Callback handling failed",
			zap.String("bot_id", s.botID.String()),
			zap.Int64("user_id", userID),
			zap.String("action", action),
			zap.Error(err))
	} else {
		s.logger.Debug("Callback handling succeeded",
			zap.String("bot_id", s.botID.String()),
			zap.Int64("user_id", userID),
			zap.String("action", action))
	}
	return err
}
