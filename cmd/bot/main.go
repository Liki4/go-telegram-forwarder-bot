package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"go-telegram-forwarder-bot/internal/bot"
	"go-telegram-forwarder-bot/internal/config"
	"go-telegram-forwarder-bot/internal/database"
	"go-telegram-forwarder-bot/internal/logger"
	"go-telegram-forwarder-bot/internal/repository"
	"go-telegram-forwarder-bot/internal/service"
	"go-telegram-forwarder-bot/internal/service/blacklist"
	"go-telegram-forwarder-bot/internal/service/manager_bot"
	"go-telegram-forwarder-bot/internal/service/message"
	"go-telegram-forwarder-bot/internal/service/statistics"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// Initialize logger
	log, err := logger.New(cfg.Log)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer log.Sync()

	log.Info("Starting telegram forwarder bot")

	// Connect to database
	db, err := database.Connect(cfg.Database)
	if err != nil {
		log.Fatal("Failed to connect to database", zap.Error(err))
	}

	// Create temporary ManagerBot for error notifications (before full initialization)
	tempManagerBot, tempErr := gotgbot.NewBot(cfg.ManagerBot.Token, nil)
	if tempErr == nil {
		tempErrorNotifier := service.NewErrorNotifier(tempManagerBot, cfg, log)
		// Notify about database connection (though we already fatal, this is for future use)
		_ = tempErrorNotifier
	}

	// Run migrations
	if err := database.Migrate(db); err != nil {
		log.Fatal("Failed to run migrations", zap.Error(err))
	}

	log.Info("Database connected and migrated successfully")

	// Initialize Redis if enabled
	// According to requirements: if connection fails at startup, terminate directly
	var redisClient *redis.Client
	if cfg.Redis.Enabled {
		redisClient, err = database.ConnectRedis(cfg.Redis)
		if err != nil {
			log.Fatal("Failed to connect to Redis at startup", zap.Error(err))
		}
		log.Info("Redis connected successfully")
	}

	// Initialize repositories
	userRepo := repository.NewUserRepository(db)
	botRepo := repository.NewBotRepository(db)
	recipientRepo := repository.NewRecipientRepository(db)
	guestRepo := repository.NewGuestRepository(db)
	blacklistRepo := repository.NewBlacklistRepository(db)
	botAdminRepo := repository.NewBotAdminRepository(db)
	messageMappingRepo := repository.NewMessageMappingRepository(db)
	auditLogRepo := repository.NewAuditLogRepository(db)

	// Initialize services
	statsService := statistics.NewService(botRepo, guestRepo, messageMappingRepo, log)

	// Initialize rate limiter and retry handler
	// Rate limiter will handle nil redisClient gracefully
	rateLimiter := message.NewRateLimiter(redisClient, cfg, log)
	retryHandler := message.NewRetryHandler(cfg, log)

	// Initialize group monitor
	groupMonitor := service.NewGroupMonitor(botRepo, recipientRepo, auditLogRepo, log)

	// Initialize message forwarder
	messageForwarder := message.NewForwarder(
		botRepo,
		recipientRepo,
		guestRepo,
		messageMappingRepo,
		rateLimiter,
		retryHandler,
		cfg,
		log,
	)

	// Set group monitor for message forwarder (error notifier will be set later)
	messageForwarder.SetGroupMonitor(groupMonitor)

	// Initialize blacklist service
	blacklistService := blacklist.NewService(blacklistRepo, guestRepo, log)

	// Start blacklist auto-approve worker
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go blacklistService.StartAutoApproveWorker(ctx)

	// Initialize ManagerBot service
	managerBotService, err := manager_bot.NewService(
		botRepo,
		userRepo,
		auditLogRepo,
		statsService,
		cfg,
		log,
	)
	if err != nil {
		log.Fatal("Failed to create ManagerBot service", zap.Error(err))
	}

	// Create and start ManagerBot
	managerBotInstance, err := bot.NewManagerBot(cfg.ManagerBot.Token, managerBotService, log, cfg)
	if err != nil {
		log.Fatal("Failed to create ManagerBot", zap.Error(err))
	}

	// Initialize error notifier
	errorNotifier := service.NewErrorNotifier(managerBotInstance.GetBot(), cfg, log)

	// Set error notifier and manager notifier for message forwarder
	messageForwarder.SetErrorNotifier(errorNotifier)
	managerNotifier := service.NewManagerNotifier(managerBotInstance.GetBot(), botRepo, userRepo, log)
	messageForwarder.SetManagerNotifier(managerNotifier)

	// Monitor Redis connection in runtime (if enabled)
	// Use a pointer to allow updating redisClient in the monitor function
	redisClientPtr := &redisClient
	if cfg.Redis.Enabled {
		go monitorRedisConnection(ctx, redisClientPtr, cfg, errorNotifier, log)
	}

	// Create BotManager for dynamic bot lifecycle management
	botManager, err := bot.NewBotManager(
		ctx,
		botRepo,
		recipientRepo,
		guestRepo,
		blacklistRepo,
		botAdminRepo,
		messageMappingRepo,
		userRepo,
		auditLogRepo,
		blacklistService,
		statsService,
		groupMonitor,
		rateLimiter,
		retryHandler,
		errorNotifier,
		managerNotifier,
		cfg,
		log,
	)
	if err != nil {
		log.Fatal("Failed to create BotManager", zap.Error(err))
	}

	// Set BotManager for ManagerBot service to enable dynamic bot management
	managerBotService.SetBotManager(botManager)

	// Load all ForwarderBots from database and start them
	if err := botManager.LoadAllBots(); err != nil {
		log.Warn("Failed to load some ForwarderBots", zap.Error(err))
	}

	// Start all bots
	var wg sync.WaitGroup

	// Start ManagerBot
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := managerBotInstance.Start(ctx); err != nil {
			log.Error("ManagerBot error", zap.Error(err))
		}
	}()

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	log.Info("All bots are running. Press Ctrl+C to stop.")
	<-sigChan

	log.Info("Shutting down...")

	// Stop all bots
	cancel()
	managerBotInstance.Stop()
	botManager.StopAll()

	// Wait for all goroutines to finish
	wg.Wait()
	botManager.Wait()

	log.Info("Shutdown complete")
}

func monitorRedisConnection(
	ctx context.Context,
	redisClientPtr **redis.Client,
	cfg *config.Config,
	errorNotifier *service.ErrorNotifier,
	log *zap.Logger,
) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if *redisClientPtr == nil {
				continue
			}

			// Check Redis connection
			pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			err := (*redisClientPtr).Ping(pingCtx).Err()
			cancel()

			if err != nil {
				log.Warn("Redis connection lost, attempting to reconnect",
					zap.Error(err))

				// Try to reconnect with retry
				newClient, retryErr := database.RetryRedisConnection(cfg.Redis, 3, 10*time.Second)
				if retryErr != nil {
					log.Error("Failed to reconnect to Redis after retries",
						zap.Error(retryErr))
					errorNotifier.NotifyCriticalError(ctx, service.ErrorTypeRedis, retryErr,
						"Redis connection lost and reconnection failed after 3 retries")
					// Continue without Redis (degraded mode)
					*redisClientPtr = nil
				} else {
					log.Info("Redis reconnected successfully")
					*redisClientPtr = newClient
				}
			}
		}
	}
}
