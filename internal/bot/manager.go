package bot

import (
	"context"
	"fmt"
	"sync"

	"go-telegram-forwarder-bot/internal/config"
	"go-telegram-forwarder-bot/internal/repository"
	"go-telegram-forwarder-bot/internal/service"
	"go-telegram-forwarder-bot/internal/service/blacklist"
	"go-telegram-forwarder-bot/internal/service/forwarder_bot"
	"go-telegram-forwarder-bot/internal/service/message"
	"go-telegram-forwarder-bot/internal/service/statistics"
	"go-telegram-forwarder-bot/internal/utils"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// BotManagerParams contains all dependencies for creating a BotManager
type BotManagerParams struct {
	Ctx                          context.Context
	BotRepo                      repository.BotRepository
	RecipientRepo                repository.RecipientRepository
	GuestRepo                    repository.GuestRepository
	BlacklistRepo                repository.BlacklistRepository
	BlacklistApprovalMessageRepo repository.BlacklistApprovalMessageRepository
	BotAdminRepo                 repository.BotAdminRepository
	MessageMappingRepo           repository.MessageMappingRepository
	UserRepo                     repository.UserRepository
	AuditLogRepo                 repository.AuditLogRepository
	BlacklistService             *blacklist.Service
	StatsService                 *statistics.Service
	GroupMonitor                 *service.GroupMonitor
	RateLimiter                  *message.RateLimiter
	RetryHandler                 *message.RetryHandler
	ErrorNotifier                *service.ErrorNotifier
	ManagerNotifier              *service.ManagerNotifier
	Config                       *config.Config
	Logger                       *zap.Logger
}

// BotManager manages the lifecycle of all ForwarderBot instances
type BotManager struct {
	bots                         map[uuid.UUID]*ForwarderBot
	mu                           sync.RWMutex
	ctx                          context.Context
	botRepo                      repository.BotRepository
	recipientRepo                repository.RecipientRepository
	guestRepo                    repository.GuestRepository
	blacklistRepo                repository.BlacklistRepository
	blacklistApprovalMessageRepo repository.BlacklistApprovalMessageRepository
	botAdminRepo                 repository.BotAdminRepository
	messageMappingRepo           repository.MessageMappingRepository
	userRepo                     repository.UserRepository
	auditLogRepo                 repository.AuditLogRepository
	blacklistService             *blacklist.Service
	statsService                 *statistics.Service
	groupMonitor                 *service.GroupMonitor
	rateLimiter                  *message.RateLimiter
	retryHandler                 *message.RetryHandler
	errorNotifier                *service.ErrorNotifier
	managerNotifier              *service.ManagerNotifier
	config                       *config.Config
	logger                       *zap.Logger
	encryptionKey                []byte
	wg                           sync.WaitGroup
}

// NewBotManager creates a new BotManager instance using BotManagerParams
func NewBotManager(params BotManagerParams) (*BotManager, error) {
	encryptionKey, err := utils.GetEncryptionKeyFromConfig(params.Config.EncryptionKey, params.Config.Environment)
	if err != nil {
		return nil, fmt.Errorf("failed to get encryption key: %w", err)
	}

	return &BotManager{
		bots:                         make(map[uuid.UUID]*ForwarderBot),
		ctx:                          params.Ctx,
		botRepo:                      params.BotRepo,
		recipientRepo:                params.RecipientRepo,
		guestRepo:                    params.GuestRepo,
		blacklistRepo:                params.BlacklistRepo,
		blacklistApprovalMessageRepo: params.BlacklistApprovalMessageRepo,
		botAdminRepo:                 params.BotAdminRepo,
		messageMappingRepo:           params.MessageMappingRepo,
		userRepo:                     params.UserRepo,
		auditLogRepo:                 params.AuditLogRepo,
		blacklistService:             params.BlacklistService,
		statsService:                 params.StatsService,
		groupMonitor:                 params.GroupMonitor,
		rateLimiter:                  params.RateLimiter,
		retryHandler:                 params.RetryHandler,
		errorNotifier:                params.ErrorNotifier,
		managerNotifier:              params.ManagerNotifier,
		config:                       params.Config,
		logger:                       params.Logger,
		encryptionKey:                encryptionKey,
	}, nil
}

// LoadAllBots loads all bots from database and starts them
func (bm *BotManager) LoadAllBots() error {
	bots, err := bm.botRepo.GetAll()
	if err != nil {
		return fmt.Errorf("failed to get all bots: %w", err)
	}

	bm.logger.Debug("Loading all ForwarderBots from database",
		zap.Int("bot_count", len(bots)))

	for _, botModel := range bots {
		if err := bm.StartBot(botModel.ID); err != nil {
			bm.logger.Warn("Failed to start bot",
				zap.String("bot_id", botModel.ID.String()),
				zap.Error(err))
			// Continue loading other bots even if one fails
		}
	}

	bm.logger.Info("Loaded all ForwarderBots",
		zap.Int("total_bots", len(bm.bots)))
	return nil
}

// StartBot starts a ForwarderBot by its ID
// botID can be uuid.UUID or any type that can be converted to uuid.UUID
func (bm *BotManager) StartBot(botID interface{}) error {
	var id uuid.UUID
	switch v := botID.(type) {
	case uuid.UUID:
		id = v
	case string:
		parsed, err := uuid.Parse(v)
		if err != nil {
			return fmt.Errorf("invalid bot ID format: %w", err)
		}
		id = parsed
	default:
		return fmt.Errorf("unsupported bot ID type: %T", botID)
	}
	return bm.startBot(id)
}

func (bm *BotManager) startBot(botID uuid.UUID) error {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	// Check if bot is already running
	if _, exists := bm.bots[botID]; exists {
		bm.logger.Debug("Bot is already running",
			zap.String("bot_id", botID.String()))
		return nil
	}

	// Get bot from database
	botModel, err := bm.botRepo.GetByID(botID)
	if err != nil {
		return fmt.Errorf("failed to get bot from database: %w", err)
	}

	bm.logger.Debug("Starting ForwarderBot",
		zap.String("bot_id", botID.String()),
		zap.String("bot_name", botModel.Name))

	// Create a message forwarder instance for this bot
	botMessageForwarder := message.NewForwarder(
		bm.botRepo,
		bm.recipientRepo,
		bm.guestRepo,
		bm.messageMappingRepo,
		bm.rateLimiter,
		bm.retryHandler,
		bm.config,
		bm.logger,
	)
	botMessageForwarder.SetGroupMonitor(bm.groupMonitor)
	botMessageForwarder.SetErrorNotifier(bm.errorNotifier)
	botMessageForwarder.SetManagerNotifier(bm.managerNotifier)

	// Create ForwarderBot service
	forwarderBotService, err := forwarder_bot.NewService(
		botID,
		bm.botRepo,
		bm.recipientRepo,
		bm.guestRepo,
		bm.blacklistRepo,
		bm.blacklistApprovalMessageRepo,
		bm.botAdminRepo,
		bm.messageMappingRepo,
		bm.userRepo,
		bm.auditLogRepo,
		botMessageForwarder,
		bm.blacklistService,
		bm.statsService,
		bm.config,
		bm.logger,
	)
	if err != nil {
		return fmt.Errorf("failed to create ForwarderBot service: %w", err)
	}

	// Create ForwarderBot instance
	forwarderBot, err := NewForwarderBotFromEncrypted(
		botModel.Token,
		bm.encryptionKey,
		botID,
		forwarderBotService,
		bm.logger,
		bm.config,
	)
	if err != nil {
		return fmt.Errorf("failed to create ForwarderBot instance: %w", err)
	}

	// Start group monitoring for this bot
	botInstance := forwarderBot.GetBot()
	if botInstance != nil {
		monitorCtx := context.Background()
		go bm.groupMonitor.StartPeriodicCheck(monitorCtx, botInstance, botID)
	}

	// Store bot instance
	bm.bots[botID] = forwarderBot

	// Start bot in a goroutine
	bm.wg.Add(1)
	go func(fb *ForwarderBot) {
		defer bm.wg.Done()
		if err := fb.Start(bm.ctx); err != nil {
			bm.logger.Error("ForwarderBot error",
				zap.String("bot_id", fb.GetBotID().String()),
				zap.Error(err))
		}
	}(forwarderBot)

	bm.logger.Info("ForwarderBot started successfully",
		zap.String("bot_id", botID.String()),
		zap.String("bot_name", botModel.Name))

	return nil
}

// StopBot stops a ForwarderBot by its ID
// botID can be uuid.UUID or any type that can be converted to uuid.UUID
func (bm *BotManager) StopBot(botID interface{}) error {
	var id uuid.UUID
	switch v := botID.(type) {
	case uuid.UUID:
		id = v
	case string:
		parsed, err := uuid.Parse(v)
		if err != nil {
			return fmt.Errorf("invalid bot ID format: %w", err)
		}
		id = parsed
	default:
		return fmt.Errorf("unsupported bot ID type: %T", botID)
	}
	return bm.stopBot(id)
}

func (bm *BotManager) stopBot(botID uuid.UUID) error {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	bot, exists := bm.bots[botID]
	if !exists {
		bm.logger.Debug("Bot is not running",
			zap.String("bot_id", botID.String()))
		return nil
	}

	bm.logger.Debug("Stopping ForwarderBot",
		zap.String("bot_id", botID.String()))

	// Stop the bot
	bot.Stop()

	// Remove from map
	delete(bm.bots, botID)

	bm.logger.Info("ForwarderBot stopped successfully",
		zap.String("bot_id", botID.String()))

	return nil
}

// GetBot returns a ForwarderBot instance by ID (for read-only access)
func (bm *BotManager) GetBot(botID uuid.UUID) (*ForwarderBot, bool) {
	bm.mu.RLock()
	defer bm.mu.RUnlock()
	bot, exists := bm.bots[botID]
	return bot, exists
}

// GetAllBots returns all running ForwarderBot instances
func (bm *BotManager) GetAllBots() []*ForwarderBot {
	bm.mu.RLock()
	defer bm.mu.RUnlock()

	bots := make([]*ForwarderBot, 0, len(bm.bots))
	for _, bot := range bm.bots {
		bots = append(bots, bot)
	}
	return bots
}

// StopAll stops all running bots
func (bm *BotManager) StopAll() {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	bm.logger.Debug("Stopping all ForwarderBots",
		zap.Int("bot_count", len(bm.bots)))

	for botID, bot := range bm.bots {
		bm.logger.Debug("Stopping ForwarderBot",
			zap.String("bot_id", botID.String()))
		bot.Stop()
	}

	// Clear the map
	bm.bots = make(map[uuid.UUID]*ForwarderBot)
}

// Wait waits for all bot goroutines to finish
func (bm *BotManager) Wait() {
	bm.wg.Wait()
}
