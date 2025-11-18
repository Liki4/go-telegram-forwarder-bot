package bot

import (
	"context"
	"fmt"
	"strings"

	"go-telegram-forwarder-bot/internal/config"
	"go-telegram-forwarder-bot/internal/service/manager_bot"
	"go-telegram-forwarder-bot/internal/utils"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"go.uber.org/zap"
)

type ManagerBot struct {
	bot     *gotgbot.Bot
	updater *ext.Updater
	service *manager_bot.Service
	logger  *zap.Logger
	stop    chan struct{}
}

func NewManagerBot(token string, service *manager_bot.Service, logger *zap.Logger, cfg *config.Config) (*ManagerBot, error) {
	var botOpts *gotgbot.BotOpts

	// Create HTTP client with proxy if enabled
	if cfg.Proxy.Enabled {
		httpClient, err := utils.CreateHTTPClientWithProxy(&cfg.Proxy)
		if err != nil {
			return nil, fmt.Errorf("failed to create HTTP client with proxy: %w", err)
		}

		// Create BaseBotClient with proxy-enabled HTTP client
		botClient := &gotgbot.BaseBotClient{
			Client:             *httpClient,
			UseTestEnvironment: false,
			DefaultRequestOpts: nil,
		}

		botOpts = &gotgbot.BotOpts{
			BotClient: botClient,
		}

		logger.Info("Proxy enabled for ManagerBot", zap.String("proxy_url", cfg.Proxy.URL))
	}

	b, err := gotgbot.NewBot(token, botOpts)
	if err != nil {
		return nil, err
	}

	dispatcher := ext.NewDispatcher(&ext.DispatcherOpts{
		Processor: ext.BaseProcessor{},
	})
	updater := ext.NewUpdater(dispatcher, nil)

	return &ManagerBot{
		bot:     b,
		updater: updater,
		service: service,
		logger:  logger,
		stop:    make(chan struct{}),
	}, nil
}

func (mb *ManagerBot) Start(ctx context.Context) error {
	dispatcher := mb.updater.Dispatcher

	// Type assert to *Dispatcher to access AddHandlerToGroup
	dp, ok := dispatcher.(*ext.Dispatcher)
	if !ok {
		return fmt.Errorf("dispatcher is not *ext.Dispatcher")
	}

	// Create a handler that processes all updates
	handler := &updateHandler{
		bot:     mb.bot,
		service: mb.service,
		logger:  mb.logger,
		ctx:     ctx,
	}
	dp.AddHandlerToGroup(handler, 0)

	// Start polling
	err := mb.updater.StartPolling(mb.bot, &ext.PollingOpts{
		DropPendingUpdates: true,
	})
	if err != nil {
		return err
	}

	mb.logger.Info("ManagerBot started successfully")

	// Wait for stop signal
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-mb.stop:
		return nil
	}
}

func (mb *ManagerBot) Stop() {
	close(mb.stop)
	mb.updater.Stop()
	mb.logger.Info("ManagerBot stopped")
}

func (mb *ManagerBot) GetBot() *gotgbot.Bot {
	return mb.bot
}

type updateHandler struct {
	bot     *gotgbot.Bot
	service *manager_bot.Service
	logger  *zap.Logger
	ctx     context.Context
}

func (h *updateHandler) CheckUpdate(b *gotgbot.Bot, ctx *ext.Context) bool {
	return true
}

func (h *updateHandler) HandleUpdate(b *gotgbot.Bot, ctx *ext.Context) error {
	update := ctx.Update

	h.logger.Debug("ManagerBot update received",
		zap.Int64("update_id", update.UpdateId),
		zap.Bool("has_message", update.Message != nil),
		zap.Bool("has_callback_query", update.CallbackQuery != nil))

	// Handle callback queries
	if update.CallbackQuery != nil {
		h.logger.Debug("Processing callback query",
			zap.String("callback_id", update.CallbackQuery.Id),
			zap.String("data", update.CallbackQuery.Data),
			zap.Int64("user_id", update.CallbackQuery.From.Id),
			zap.Int64("chat_id", update.CallbackQuery.Message.GetChat().Id))
		err := h.service.HandleCallback(h.ctx, b, ctx)
		if err != nil {
			h.logger.Debug("Callback handling completed with error",
				zap.String("callback_id", update.CallbackQuery.Id),
				zap.Error(err))
		} else {
			h.logger.Debug("Callback handling completed successfully",
				zap.String("callback_id", update.CallbackQuery.Id))
		}
		return err
	}

	// Handle messages
	if update.Message != nil {
		message := update.Message
		text := message.Text
		userID := message.From.Id
		chatID := message.Chat.Id

		h.logger.Debug("ManagerBot message received",
			zap.Int64("message_id", message.MessageId),
			zap.Int64("user_id", userID),
			zap.Int64("chat_id", chatID),
			zap.String("text", text),
			zap.Bool("is_command", text != "" && strings.HasPrefix(text, "/")))

		if text != "" && strings.HasPrefix(text, "/") {
			h.logger.Debug("Processing command",
				zap.Int64("user_id", userID),
				zap.Int64("chat_id", chatID),
				zap.String("command", text))
			err := h.service.HandleCommand(h.ctx, b, ctx)
			if err != nil {
				h.logger.Debug("Command handling completed with error",
					zap.Int64("user_id", userID),
					zap.String("command", text),
					zap.Error(err))
			} else {
				h.logger.Debug("Command handling completed successfully",
					zap.Int64("user_id", userID),
					zap.String("command", text))
			}
			return err
		}
	}

	return nil
}

func (h *updateHandler) Name() string {
	return "manager_bot_handler"
}
