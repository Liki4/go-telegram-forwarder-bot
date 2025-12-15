package manager_bot

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"go-telegram-forwarder-bot/internal/config"
	"go-telegram-forwarder-bot/internal/repository"
	"go-telegram-forwarder-bot/internal/service/statistics"
	"go-telegram-forwarder-bot/internal/utils"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// BotManagerInterface defines the interface for managing ForwarderBot lifecycle
type BotManagerInterface interface {
	StartBot(botID interface{}) error
	StopBot(botID interface{}) error
}

type Service struct {
	db            *gorm.DB
	botRepo       repository.BotRepository
	userRepo      repository.UserRepository
	auditLogRepo  repository.AuditLogRepository
	recipientRepo repository.RecipientRepository
	statsService  *statistics.Service
	config        *config.Config
	logger        *zap.Logger
	encryptionKey []byte
	botManager    BotManagerInterface
	commandsCache sync.Map // Cache to track users whose commands have been updated
}

func NewService(
	db *gorm.DB,
	botRepo repository.BotRepository,
	userRepo repository.UserRepository,
	auditLogRepo repository.AuditLogRepository,
	recipientRepo repository.RecipientRepository,
	statsService *statistics.Service,
	cfg *config.Config,
	logger *zap.Logger,
) (*Service, error) {
	key, err := utils.GetEncryptionKeyFromConfig(cfg.EncryptionKey, cfg.Environment)
	if err != nil {
		return nil, fmt.Errorf("failed to get encryption key: %w", err)
	}

	return &Service{
		db:            db,
		botRepo:       botRepo,
		userRepo:      userRepo,
		auditLogRepo:  auditLogRepo,
		recipientRepo: recipientRepo,
		statsService:  statsService,
		config:        cfg,
		logger:        logger,
		encryptionKey: key,
		botManager:    nil, // Will be set via SetBotManager
	}, nil
}

// SetBotManager sets the BotManager interface for dynamic bot management
func (s *Service) SetBotManager(botManager BotManagerInterface) {
	s.botManager = botManager
}

// updateCommands updates the command menu for all users (global commands)
func (s *Service) updateCommands(_ context.Context, b *gotgbot.Bot) {
	// Check cache to avoid frequent API calls
	if _, exists := s.commandsCache.Load("commands_set"); exists {
		return
	}

	// Include all commands for all users
	var commands []gotgbot.BotCommand
	commands = append(commands, gotgbot.BotCommand{
		Command:     "help",
		Description: "Show help message",
	})
	commands = append(commands, gotgbot.BotCommand{
		Command:     "addbot",
		Description: "Register a new ForwarderBot",
	})
	commands = append(commands, gotgbot.BotCommand{
		Command:     "mybots",
		Description: "List all your ForwarderBots",
	})
	commands = append(commands, gotgbot.BotCommand{
		Command:     "manage",
		Description: "Open management menu",
	})
	commands = append(commands, gotgbot.BotCommand{
		Command:     "stats",
		Description: "View global statistics",
	})

	// Set commands for private chats (default scope)
	scope := gotgbot.BotCommandScopeDefault{}
	opts := &gotgbot.SetMyCommandsOpts{
		Scope: scope,
	}

	_, err := b.SetMyCommands(commands, opts)
	if err != nil {
		s.logger.Warn("Failed to set commands for private chats",
			zap.Error(err))
		return
	}

	// Set commands for group chats
	groupScope := gotgbot.BotCommandScopeAllGroupChats{}
	groupOpts := &gotgbot.SetMyCommandsOpts{
		Scope: groupScope,
	}

	_, err = b.SetMyCommands(commands, groupOpts)
	if err != nil {
		s.logger.Warn("Failed to set commands for group chats",
			zap.Error(err))
		// Continue anyway, as private chat commands are already set
	}

	// Set global menu button to show commands (no chatID = global)
	menuButton := gotgbot.MenuButtonCommands{}
	_, err = b.SetChatMenuButton(&gotgbot.SetChatMenuButtonOpts{
		MenuButton: menuButton,
	})
	if err != nil {
		s.logger.Warn("Failed to set global menu button",
			zap.Error(err))
		// Don't return, as commands are already set
	}

	// Cache the update
	s.commandsCache.Store("commands_set", true)
	s.logger.Debug("Commands and menu button updated globally",
		zap.Int("command_count", len(commands)))
}

func (s *Service) IsSuperuser(userID int64) bool {
	s.logger.Debug("Checking superuser status",
		zap.Int64("user_id", userID),
		zap.Int64s("superusers", s.config.ManagerBot.Superusers))
	for _, superuserID := range s.config.ManagerBot.Superusers {
		if superuserID == userID {
			s.logger.Debug("User is superuser",
				zap.Int64("user_id", userID))
			return true
		}
	}
	s.logger.Debug("User is not superuser",
		zap.Int64("user_id", userID))
	return false
}

// IsBotManager checks if a user is the manager of a specific bot
func (s *Service) IsBotManager(userID int64, botID uuid.UUID) (bool, error) {
	s.logger.Debug("Checking if user is bot manager",
		zap.Int64("user_id", userID),
		zap.String("bot_id", botID.String()))

	bot, err := s.botRepo.GetByID(botID)
	if err != nil {
		s.logger.Debug("Failed to get bot for manager check",
			zap.Int64("user_id", userID),
			zap.String("bot_id", botID.String()),
			zap.Error(err))
		return false, err
	}

	user, err := s.userRepo.GetByTelegramUserID(userID)
	if err != nil {
		s.logger.Debug("Failed to get user for manager check",
			zap.Int64("user_id", userID),
			zap.String("bot_id", botID.String()),
			zap.Error(err))
		return false, err
	}

	isManager := user.ID == bot.ManagerID
	s.logger.Debug("Bot manager check result",
		zap.Int64("user_id", userID),
		zap.String("bot_id", botID.String()),
		zap.Bool("is_manager", isManager),
		zap.String("user_uuid", user.ID.String()),
		zap.String("bot_manager_uuid", bot.ManagerID.String()))
	return isManager, nil
}

func (s *Service) HandleCommand(ctx context.Context, b *gotgbot.Bot, update *ext.Context) error {
	userID := update.EffectiveUser.Id
	chatID := update.EffectiveChat.Id
	command := update.EffectiveMessage.Text

	// Update commands menu (global, only once)
	s.updateCommands(ctx, b)

	s.logger.Debug("ManagerBot command received",
		zap.Int64("user_id", userID),
		zap.Int64("chat_id", chatID),
		zap.String("command", command))

	switch {
	case strings.HasPrefix(command, "/help"):
		s.logger.Debug("Handling /help command",
			zap.Int64("user_id", userID),
			zap.Int64("chat_id", chatID))
		err := s.handleHelp(ctx, b, update)
		if err != nil {
			s.logger.Debug("/help command failed",
				zap.Int64("user_id", userID),
				zap.Error(err))
		} else {
			s.logger.Debug("/help command succeeded",
				zap.Int64("user_id", userID))
		}
		return err
	case strings.HasPrefix(command, "/addbot"):
		s.logger.Debug("Handling /addbot command",
			zap.Int64("user_id", userID),
			zap.Int64("chat_id", chatID))
		err := s.handleAddBot(ctx, b, update)
		if err != nil {
			s.logger.Debug("/addbot command failed",
				zap.Int64("user_id", userID),
				zap.Error(err))
		} else {
			s.logger.Debug("/addbot command succeeded",
				zap.Int64("user_id", userID))
		}
		return err
	case strings.HasPrefix(command, "/mybots"):
		s.logger.Debug("Handling /mybots command",
			zap.Int64("user_id", userID),
			zap.Int64("chat_id", chatID))
		err := s.handleMyBots(ctx, b, update)
		if err != nil {
			s.logger.Debug("/mybots command failed",
				zap.Int64("user_id", userID),
				zap.Error(err))
		} else {
			s.logger.Debug("/mybots command succeeded",
				zap.Int64("user_id", userID))
		}
		return err
	case strings.HasPrefix(command, "/manage"):
		s.logger.Debug("Handling /manage command",
			zap.Int64("user_id", userID),
			zap.Int64("chat_id", chatID))
		if !s.IsSuperuser(userID) {
			s.logger.Debug("Access denied for /manage command",
				zap.Int64("user_id", userID))
			_, err := b.SendMessage(update.EffectiveChat.Id, "You are not authorized to use this command.", nil)
			return err
		}
		err := s.handleManage(ctx, b, update)
		if err != nil {
			s.logger.Debug("/manage command failed",
				zap.Int64("user_id", userID),
				zap.Error(err))
		} else {
			s.logger.Debug("/manage command succeeded",
				zap.Int64("user_id", userID))
		}
		return err
	case strings.HasPrefix(command, "/stats"):
		s.logger.Debug("Handling /stats command",
			zap.Int64("user_id", userID),
			zap.Int64("chat_id", chatID))
		if !s.IsSuperuser(userID) {
			s.logger.Debug("Access denied for /stats command",
				zap.Int64("user_id", userID))
			_, err := b.SendMessage(update.EffectiveChat.Id, "You are not authorized to use this command.", nil)
			return err
		}
		err := s.handleStats(ctx, b, update)
		if err != nil {
			s.logger.Debug("/stats command failed",
				zap.Int64("user_id", userID),
				zap.Error(err))
		} else {
			s.logger.Debug("/stats command succeeded",
				zap.Int64("user_id", userID))
		}
		return err
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
	chatID := update.EffectiveChat.Id
	data := update.CallbackQuery.Data
	parts := strings.Split(data, ":")

	s.logger.Debug("ManagerBot callback received",
		zap.Int64("user_id", userID),
		zap.Int64("chat_id", chatID),
		zap.String("callback_data", data),
		zap.Strings("parts", parts),
		zap.Int("parts_count", len(parts)))

	if len(parts) < 2 {
		s.logger.Debug("Invalid callback data format",
			zap.Int64("user_id", userID),
			zap.String("callback_data", data),
			zap.Int("parts_count", len(parts)))
		return fmt.Errorf("invalid callback data: %s", data)
	}

	action := parts[0]
	s.logger.Debug("Processing callback action",
		zap.Int64("user_id", userID),
		zap.String("action", action),
		zap.Strings("parts", parts))

	var err error
	switch action {
	case "manage":
		// Only superusers can access manage callbacks
		if !s.IsSuperuser(userID) {
			s.logger.Debug("Access denied for manage callback",
				zap.Int64("user_id", userID))
			_, err := b.AnswerCallbackQuery(update.CallbackQuery.Id, &gotgbot.AnswerCallbackQueryOpts{
				Text: "You are not authorized to access this.",
			})
			return err
		}
		s.logger.Debug("Handling manage callback",
			zap.Int64("user_id", userID),
			zap.Strings("sub_parts", parts[1:]))
		err = s.handleManageCallback(ctx, b, update, parts[1:])
	case "bot":
		s.logger.Debug("Handling bot callback",
			zap.Int64("user_id", userID),
			zap.Strings("sub_parts", parts[1:]))
		err = s.handleBotCallback(ctx, b, update, parts[1:])
	case "manager":
		// Only superusers can access manager callbacks
		if !s.IsSuperuser(userID) {
			s.logger.Debug("Access denied for manager callback",
				zap.Int64("user_id", userID))
			_, err := b.AnswerCallbackQuery(update.CallbackQuery.Id, &gotgbot.AnswerCallbackQueryOpts{
				Text: "You are not authorized to access this.",
			})
			return err
		}
		s.logger.Debug("Handling manager callback",
			zap.Int64("user_id", userID),
			zap.Strings("sub_parts", parts[1:]))
		err = s.handleManagerCallback(ctx, b, update, parts[1:])
	case "delete_bot":
		s.logger.Debug("Handling delete_bot callback",
			zap.Int64("user_id", userID),
			zap.Strings("sub_parts", parts[1:]))
		err = s.handleDeleteBotCallback(ctx, b, update, parts[1:])
	case "mybots":
		// Handle mybots callback to return to /mybots list
		// Only allow "list" action for now
		if len(parts) > 1 && parts[1] == "list" {
			s.logger.Debug("Handling mybots callback",
				zap.Int64("user_id", userID),
				zap.Strings("sub_parts", parts[1:]))
			err = s.handleMyBotsCallback(ctx, b, update)
		} else {
			s.logger.Debug("Invalid mybots callback",
				zap.Int64("user_id", userID),
				zap.Strings("parts", parts))
			_, err := b.AnswerCallbackQuery(update.CallbackQuery.Id, &gotgbot.AnswerCallbackQueryOpts{
				Text: "Invalid callback data",
			})
			return err
		}
	default:
		s.logger.Debug("Unknown callback action",
			zap.Int64("user_id", userID),
			zap.String("action", action))
		err = fmt.Errorf("unknown callback action: %s", action)
	}

	if err != nil {
		s.logger.Debug("Callback handling failed",
			zap.Int64("user_id", userID),
			zap.String("action", action),
			zap.Error(err))
	} else {
		s.logger.Debug("Callback handling succeeded",
			zap.Int64("user_id", userID),
			zap.String("action", action))
	}
	return err
}
