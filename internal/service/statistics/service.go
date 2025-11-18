package statistics

import (
	"github.com/google/uuid"
	"go-telegram-forwarder-bot/internal/models"
	"go-telegram-forwarder-bot/internal/repository"
	"go.uber.org/zap"
)

type Service struct {
	botRepo            repository.BotRepository
	guestRepo          repository.GuestRepository
	messageMappingRepo repository.MessageMappingRepository
	logger             *zap.Logger
}

type GlobalStatistics struct {
	ManagerCount    int64
	BotCount        int64
	TotalInbound    int64
	TotalOutbound   int64
	TotalGuestCount int64
}

type BotStatistics struct {
	BotID         uuid.UUID
	BotName       string
	InboundCount  int64
	OutboundCount int64
	GuestCount    int64
}

type ManagerStatistics struct {
	Bots []BotStatistics
}

func NewService(
	botRepo repository.BotRepository,
	guestRepo repository.GuestRepository,
	messageMappingRepo repository.MessageMappingRepository,
	logger *zap.Logger,
) *Service {
	return &Service{
		botRepo:            botRepo,
		guestRepo:          guestRepo,
		messageMappingRepo: messageMappingRepo,
		logger:             logger,
	}
}

func (s *Service) GetGlobalStatistics() (*GlobalStatistics, error) {
	bots, err := s.botRepo.GetAll()
	if err != nil {
		return nil, err
	}

	managerMap := make(map[uuid.UUID]bool)
	var totalInbound, totalOutbound, totalGuestCount int64

	for _, bot := range bots {
		managerMap[bot.ManagerID] = true

		inbound, err := s.messageMappingRepo.CountByBotIDAndDirection(
			bot.ID, models.MessageDirectionInbound)
		if err != nil {
			s.logger.Warn("Failed to count inbound messages",
				zap.String("bot_id", bot.ID.String()),
				zap.Error(err))
			continue
		}
		totalInbound += inbound

		outbound, err := s.messageMappingRepo.CountByBotIDAndDirection(
			bot.ID, models.MessageDirectionOutbound)
		if err != nil {
			s.logger.Warn("Failed to count outbound messages",
				zap.String("bot_id", bot.ID.String()),
				zap.Error(err))
			continue
		}
		totalOutbound += outbound

		guestCount, err := s.guestRepo.CountByBotID(bot.ID)
		if err != nil {
			s.logger.Warn("Failed to count guests",
				zap.String("bot_id", bot.ID.String()),
				zap.Error(err))
			continue
		}
		totalGuestCount += guestCount
	}

	return &GlobalStatistics{
		ManagerCount:    int64(len(managerMap)),
		BotCount:        int64(len(bots)),
		TotalInbound:    totalInbound,
		TotalOutbound:   totalOutbound,
		TotalGuestCount: totalGuestCount,
	}, nil
}

func (s *Service) GetManagerStatistics(managerID uuid.UUID) (*ManagerStatistics, error) {
	bots, err := s.botRepo.GetByManagerID(managerID)
	if err != nil {
		return nil, err
	}

	botStats := make([]BotStatistics, 0, len(bots))
	for _, bot := range bots {
		inbound, err := s.messageMappingRepo.CountByBotIDAndDirection(
			bot.ID, models.MessageDirectionInbound)
		if err != nil {
			s.logger.Warn("Failed to count inbound messages",
				zap.String("bot_id", bot.ID.String()),
				zap.Error(err))
			inbound = 0
		}

		outbound, err := s.messageMappingRepo.CountByBotIDAndDirection(
			bot.ID, models.MessageDirectionOutbound)
		if err != nil {
			s.logger.Warn("Failed to count outbound messages",
				zap.String("bot_id", bot.ID.String()),
				zap.Error(err))
			outbound = 0
		}

		guestCount, err := s.guestRepo.CountByBotID(bot.ID)
		if err != nil {
			s.logger.Warn("Failed to count guests",
				zap.String("bot_id", bot.ID.String()),
				zap.Error(err))
			guestCount = 0
		}

		botStats = append(botStats, BotStatistics{
			BotID:         bot.ID,
			BotName:       bot.Name,
			InboundCount:  inbound,
			OutboundCount: outbound,
			GuestCount:    guestCount,
		})
	}

	return &ManagerStatistics{
		Bots: botStats,
	}, nil
}

func (s *Service) GetBotStatistics(botID uuid.UUID) (*BotStatistics, error) {
	bot, err := s.botRepo.GetByID(botID)
	if err != nil {
		return nil, err
	}

	inbound, err := s.messageMappingRepo.CountByBotIDAndDirection(
		botID, models.MessageDirectionInbound)
	if err != nil {
		return nil, err
	}

	outbound, err := s.messageMappingRepo.CountByBotIDAndDirection(
		botID, models.MessageDirectionOutbound)
	if err != nil {
		return nil, err
	}

	guestCount, err := s.guestRepo.CountByBotID(botID)
	if err != nil {
		return nil, err
	}

	return &BotStatistics{
		BotID:         botID,
		BotName:       bot.Name,
		InboundCount:  inbound,
		OutboundCount: outbound,
		GuestCount:    guestCount,
	}, nil
}
