package repository

import (
	"github.com/google/uuid"
	"go-telegram-forwarder-bot/internal/models"
	"gorm.io/gorm"
)

type BotRepository interface {
	Create(bot *models.ForwarderBot) error
	GetByID(id uuid.UUID) (*models.ForwarderBot, error)
	GetByManagerID(managerID uuid.UUID) ([]*models.ForwarderBot, error)
	GetAll() ([]*models.ForwarderBot, error)
	Update(bot *models.ForwarderBot) error
	Delete(id uuid.UUID) error
	GetByToken(token string) (*models.ForwarderBot, error)
}

type botRepository struct {
	db *gorm.DB
}

func NewBotRepository(db *gorm.DB) BotRepository {
	return &botRepository{db: db}
}

func (r *botRepository) Create(bot *models.ForwarderBot) error {
	return r.db.Create(bot).Error
}

func (r *botRepository) GetByID(id uuid.UUID) (*models.ForwarderBot, error) {
	var bot models.ForwarderBot
	if err := r.db.Preload("Manager").First(&bot, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &bot, nil
}

func (r *botRepository) GetByManagerID(managerID uuid.UUID) ([]*models.ForwarderBot, error) {
	var bots []*models.ForwarderBot
	if err := r.db.Where("manager_id = ?", managerID).Find(&bots).Error; err != nil {
		return nil, err
	}
	return bots, nil
}

func (r *botRepository) GetAll() ([]*models.ForwarderBot, error) {
	var bots []*models.ForwarderBot
	if err := r.db.Preload("Manager").Find(&bots).Error; err != nil {
		return nil, err
	}
	return bots, nil
}

func (r *botRepository) Update(bot *models.ForwarderBot) error {
	return r.db.Save(bot).Error
}

func (r *botRepository) Delete(id uuid.UUID) error {
	return r.db.Delete(&models.ForwarderBot{}, "id = ?", id).Error
}

func (r *botRepository) GetByToken(token string) (*models.ForwarderBot, error) {
	var bot models.ForwarderBot
	if err := r.db.Where("token = ?", token).First(&bot).Error; err != nil {
		return nil, err
	}
	return &bot, nil
}
