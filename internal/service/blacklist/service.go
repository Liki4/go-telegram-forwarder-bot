package blacklist

import (
	"context"
	"errors"
	"time"

	"go-telegram-forwarder-bot/internal/models"
	"go-telegram-forwarder-bot/internal/repository"

	"github.com/google/uuid"
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
		// If guest doesn't exist, they are not blacklisted
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}

	// Get all blacklist records for this guest, ordered by created_at DESC
	allBlacklists, err := s.blacklistRepo.GetAllByBotIDAndGuestID(botID, guest.ID)
	if err != nil {
		return false, err
	}

	// If no blacklist records exist, user is not blacklisted
	if len(allBlacklists) == 0 {
		return false, nil
	}

	// Find the latest approved unban and the latest active ban (approved or pending)
	// The logic: if there's an approved unban that is newer than any active ban, user is not blacklisted
	var latestApprovedUnban *models.Blacklist
	var latestActiveBan *models.Blacklist

	for _, bl := range allBlacklists {
		// Check for approved unban
		if bl.RequestType == models.BlacklistRequestTypeUnban &&
			bl.Status == models.BlacklistStatusApproved {
			if latestApprovedUnban == nil || bl.CreatedAt.After(latestApprovedUnban.CreatedAt) {
				latestApprovedUnban = bl
			}
		}

		// Check for active ban (approved or pending)
		if bl.RequestType == models.BlacklistRequestTypeBan &&
			(bl.Status == models.BlacklistStatusApproved || bl.Status == models.BlacklistStatusPending) {
			if latestActiveBan == nil || bl.CreatedAt.After(latestActiveBan.CreatedAt) {
				latestActiveBan = bl
			}
		}
	}

	// If there's an approved unban and it's newer than any active ban, user is not blacklisted
	if latestApprovedUnban != nil {
		if latestActiveBan == nil || latestApprovedUnban.CreatedAt.After(latestActiveBan.CreatedAt) {
			s.logger.Debug("User is not blacklisted due to approved unban",
				zap.String("bot_id", botID.String()),
				zap.String("guest_id", guest.ID.String()),
				zap.Time("unban_created_at", latestApprovedUnban.CreatedAt))
			return false, nil
		}
	}

	// If there's an active ban, user is blacklisted
	if latestActiveBan != nil {
		s.logger.Debug("User is blacklisted due to active ban",
			zap.String("bot_id", botID.String()),
			zap.String("guest_id", guest.ID.String()),
			zap.String("ban_status", string(latestActiveBan.Status)),
			zap.Time("ban_created_at", latestActiveBan.CreatedAt))
		return true, nil
	}

	// No active ban found, user is not blacklisted
	s.logger.Debug("User is not blacklisted",
		zap.String("bot_id", botID.String()),
		zap.String("guest_id", guest.ID.String()))
	return false, nil
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
