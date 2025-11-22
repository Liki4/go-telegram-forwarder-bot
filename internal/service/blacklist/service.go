package blacklist

import (
	"context"
	"errors"
	"fmt"
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

	// Performance optimization: Only query the latest record instead of all records
	// This avoids loading potentially thousands of historical records into memory
	latest, err := s.blacklistRepo.GetLatestByBotIDAndGuestID(botID, guest.ID)
	if err != nil {
		// If no record found, user is not blacklisted
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}

	// Ban logic: approved or pending → blacklisted, rejected → not blacklisted
	if latest.RequestType == models.BlacklistRequestTypeBan {
		if latest.Status == models.BlacklistStatusApproved || latest.Status == models.BlacklistStatusPending {
			s.logger.Debug("User is blacklisted due to ban",
				zap.String("bot_id", botID.String()),
				zap.String("guest_id", guest.ID.String()),
				zap.String("ban_status", string(latest.Status)),
				zap.Time("ban_created_at", latest.CreatedAt))
			return true, nil
		}
		// Ban rejected → not blacklisted
		s.logger.Debug("User is not blacklisted (ban rejected)",
			zap.String("bot_id", botID.String()),
			zap.String("guest_id", guest.ID.String()))
		return false, nil
	}

	// Unban logic: rejected or pending → blacklisted, approved → not blacklisted
	if latest.RequestType == models.BlacklistRequestTypeUnban {
		if latest.Status == models.BlacklistStatusRejected || latest.Status == models.BlacklistStatusPending {
			s.logger.Debug("User is blacklisted (unban rejected or pending)",
				zap.String("bot_id", botID.String()),
				zap.String("guest_id", guest.ID.String()),
				zap.String("unban_status", string(latest.Status)),
				zap.Time("unban_created_at", latest.CreatedAt))
			return true, nil
		}
		// Unban approved → not blacklisted
		s.logger.Debug("User is not blacklisted (unban approved)",
			zap.String("bot_id", botID.String()),
			zap.String("guest_id", guest.ID.String()),
			zap.Time("unban_created_at", latest.CreatedAt))
		return false, nil
	}

	// Should not reach here, but default to not blacklisted
	s.logger.Warn("Unexpected blacklist record type",
		zap.String("bot_id", botID.String()),
		zap.String("guest_id", guest.ID.String()),
		zap.String("request_type", string(latest.RequestType)),
		zap.String("status", string(latest.Status)))
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

	// Check if ban can be triggered based on latest state
	// Can trigger ban if: latest is ban (pending/rejected) or unban (approved)
	latest, err := s.blacklistRepo.GetLatestByBotIDAndGuestID(botID, guest.ID)
	if err == nil && latest != nil {
		canTrigger := false
		if latest.RequestType == models.BlacklistRequestTypeBan {
			// Can trigger if ban is pending or rejected
			if latest.Status == models.BlacklistStatusPending || latest.Status == models.BlacklistStatusRejected {
				canTrigger = true
			}
		} else if latest.RequestType == models.BlacklistRequestTypeUnban {
			// Can trigger if unban is approved
			if latest.Status == models.BlacklistStatusApproved {
				canTrigger = true
			}
		}

		if !canTrigger {
			return nil, fmt.Errorf("cannot trigger ban: latest state is %s %s, which does not allow ban request", latest.RequestType, latest.Status)
		}
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

	// Check if unban can be triggered based on latest state
	// Can trigger unban if: latest is unban (rejected/pending) or ban (approved)
	latest, err := s.blacklistRepo.GetLatestByBotIDAndGuestID(botID, guest.ID)
	if err == nil && latest != nil {
		canTrigger := false
		if latest.RequestType == models.BlacklistRequestTypeUnban {
			// Can trigger if unban is rejected or pending
			if latest.Status == models.BlacklistStatusRejected || latest.Status == models.BlacklistStatusPending {
				canTrigger = true
			}
		} else if latest.RequestType == models.BlacklistRequestTypeBan {
			// Can trigger if ban is approved
			if latest.Status == models.BlacklistStatusApproved {
				canTrigger = true
			}
		}

		if !canTrigger {
			return nil, fmt.Errorf("cannot trigger unban: latest state is %s %s, which does not allow unban request", latest.RequestType, latest.Status)
		}
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
