package bot

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"go-telegram-forwarder-bot/internal/config"
	"go-telegram-forwarder-bot/internal/service/forwarder_bot"
	"go-telegram-forwarder-bot/internal/utils"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type ForwarderBot struct {
	botID   uuid.UUID
	bot     *gotgbot.Bot
	updater *ext.Updater
	service *forwarder_bot.Service
	logger  *zap.Logger
	stop    chan struct{}
	stopOnce sync.Once
}

func NewForwarderBot(token string, botID uuid.UUID, service *forwarder_bot.Service, logger *zap.Logger, cfg *config.Config) (*ForwarderBot, error) {
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

		logger.Info("Proxy enabled for ForwarderBot",
			zap.String("bot_id", botID.String()),
			zap.String("proxy_url", cfg.Proxy.URL))
	}

	b, err := gotgbot.NewBot(token, botOpts)
	if err != nil {
		return nil, err
	}

	dispatcher := ext.NewDispatcher(&ext.DispatcherOpts{
		Processor: ext.BaseProcessor{},
	})
	updater := ext.NewUpdater(dispatcher, nil)

	return &ForwarderBot{
		botID:   botID,
		bot:     b,
		updater: updater,
		service: service,
		logger:  logger,
		stop:    make(chan struct{}),
	}, nil
}

func NewForwarderBotFromEncrypted(encryptedToken string, encryptionKey []byte, botID uuid.UUID, service *forwarder_bot.Service, logger *zap.Logger, cfg *config.Config) (*ForwarderBot, error) {
	token, err := utils.DecryptToken(encryptedToken, encryptionKey)
	if err != nil {
		return nil, err
	}

	return NewForwarderBot(token, botID, service, logger, cfg)
}

func (fb *ForwarderBot) Start(ctx context.Context) error {
	dispatcher := fb.updater.Dispatcher

	// Type assert to *Dispatcher to access AddHandlerToGroup
	dp, ok := dispatcher.(*ext.Dispatcher)
	if !ok {
		return fmt.Errorf("dispatcher is not *ext.Dispatcher")
	}

	// Create a handler that processes all updates
	handler := &forwarderUpdateHandler{
		bot:     fb.bot,
		service: fb.service,
		logger:  fb.logger,
		ctx:     ctx,
	}
	dp.AddHandlerToGroup(handler, 0)

	// Start polling
	err := fb.updater.StartPolling(fb.bot, &ext.PollingOpts{
		DropPendingUpdates: true,
	})
	if err != nil {
		return err
	}

	fb.logger.Info("ForwarderBot started successfully",
		zap.String("bot_id", fb.botID.String()))

	// Wait for stop signal
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-fb.stop:
		return nil
	}
}

func (fb *ForwarderBot) Stop() {
	fb.stopOnce.Do(func() {
		close(fb.stop)
		fb.updater.Stop()
		fb.logger.Info("ForwarderBot stopped",
			zap.String("bot_id", fb.botID.String()))
	})
}

func (fb *ForwarderBot) GetBotID() uuid.UUID {
	return fb.botID
}

func (fb *ForwarderBot) GetBot() *gotgbot.Bot {
	return fb.bot
}

type forwarderUpdateHandler struct {
	bot     *gotgbot.Bot
	service *forwarder_bot.Service
	logger  *zap.Logger
	ctx     context.Context
}

func (h *forwarderUpdateHandler) CheckUpdate(b *gotgbot.Bot, ctx *ext.Context) bool {
	return true
}

func (h *forwarderUpdateHandler) HandleUpdate(b *gotgbot.Bot, ctx *ext.Context) error {
	update := ctx.Update

	h.logger.Debug("ForwarderBot update received",
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
		messageID := message.MessageId

		h.logger.Debug("ForwarderBot message received",
			zap.Int64("message_id", messageID),
			zap.Int64("user_id", userID),
			zap.Int64("chat_id", chatID),
			zap.String("text", text),
			zap.Bool("is_reply", message.ReplyToMessage != nil),
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

		h.logger.Debug("Processing regular message",
			zap.Int64("message_id", messageID),
			zap.Int64("user_id", userID),
			zap.Int64("chat_id", chatID),
			zap.Bool("is_reply", message.ReplyToMessage != nil))
		err := h.service.HandleMessage(h.ctx, b, ctx)
		if err != nil {
			h.logger.Debug("Message handling completed with error",
				zap.Int64("message_id", messageID),
				zap.Int64("user_id", userID),
				zap.Error(err))
		} else {
			h.logger.Debug("Message handling completed successfully",
				zap.Int64("message_id", messageID),
				zap.Int64("user_id", userID))
		}
		return err
	}

	return nil
}

func (h *forwarderUpdateHandler) Name() string {
	return "forwarder_bot_handler"
}
