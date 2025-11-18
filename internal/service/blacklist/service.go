package blacklist

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"go-telegram-forwarder-bot/internal/models"
	"go-telegram-forwarder-bot/internal/repository"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type Service struct {
	blacklistRepo repository.BlacklistRepository
	guestRepo     repository.GuestRepository
	logger        *zap.Logger
}

func NewService(
	blacklistRepo repository.BlacklistRepository,
	guestRepo repository.GuestRepository,
	logger *zap.Logger,
) *Service {
	return &Service{
		blacklistRepo: blacklistRepo,
		guestRepo:     guestRepo,
		logger:        logger,
	}
}

func (s *Service) IsBlacklisted(botID uuid.UUID, guestUserID int64) (bool, error) {
	guest, err := s.guestRepo.GetByBotIDAndUserID(botID, guestUserID)
	if err != nil {
		return false, err
	}

	blacklist, err := s.blacklistRepo.GetActiveByBotIDAndGuestID(botID, guest.ID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}

	return blacklist != nil && blacklist.Status == models.BlacklistStatusApproved, nil
}

func (s *Service) CreateBanRequest(
	botID uuid.UUID,
	guestUserID int64,
	requestUserID uuid.UUID,
) (*models.Blacklist, error) {
	guest, err := s.guestRepo.GetOrCreateByBotIDAndUserID(botID, guestUserID)
	if err != nil {
		return nil, err
	}

	blacklist := &models.Blacklist{
		BotID:         botID,
		GuestID:       guest.ID,
		Status:        models.BlacklistStatusPending,
		RequestUserID: requestUserID,
		RequestType:   models.BlacklistRequestTypeBan,
	}

	if err := s.blacklistRepo.Create(blacklist); err != nil {
		return nil, err
	}

	return blacklist, nil
}

func (s *Service) CreateUnbanRequest(
	botID uuid.UUID,
	guestUserID int64,
	requestUserID uuid.UUID,
) (*models.Blacklist, error) {
	// Get or create guest (guest might not exist if never sent a message)
	guest, err := s.guestRepo.GetOrCreateByBotIDAndUserID(botID, guestUserID)
	if err != nil {
		return nil, err
	}

	blacklist := &models.Blacklist{
		BotID:         botID,
		GuestID:       guest.ID,
		Status:        models.BlacklistStatusPending,
		RequestUserID: requestUserID,
		RequestType:   models.BlacklistRequestTypeUnban,
	}

	if err := s.blacklistRepo.Create(blacklist); err != nil {
		return nil, err
	}

	return blacklist, nil
}

func (s *Service) ApproveRequest(blacklistID uuid.UUID) error {
	return s.blacklistRepo.ApprovePending(blacklistID)
}

func (s *Service) RejectRequest(blacklistID uuid.UUID) error {
	return s.blacklistRepo.RejectPending(blacklistID)
}

func (s *Service) GetPendingRequests(botID uuid.UUID) ([]*models.Blacklist, error) {
	return s.blacklistRepo.GetPendingByBotID(botID)
}

func (s *Service) AutoApproveExpired(ctx context.Context) error {
	return s.blacklistRepo.AutoApproveExpired()
}

func (s *Service) StartAutoApproveWorker(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.AutoApproveExpired(ctx); err != nil {
				s.logger.Error("Failed to auto-approve expired blacklist requests",
					zap.Error(err))
			}
		}
	}
}
