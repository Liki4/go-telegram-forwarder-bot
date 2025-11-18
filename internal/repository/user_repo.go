package repository

import (
	"github.com/google/uuid"
	"go-telegram-forwarder-bot/internal/models"
	"gorm.io/gorm"
)

type UserRepository interface {
	Create(user *models.User) error
	GetByID(id uuid.UUID) (*models.User, error)
	GetByTelegramUserID(telegramUserID int64) (*models.User, error)
	GetOrCreateByTelegramUserID(telegramUserID int64, username *string) (*models.User, error)
	Update(user *models.User) error
	Delete(id uuid.UUID) error
}

type userRepository struct {
	db *gorm.DB
}

func NewUserRepository(db *gorm.DB) UserRepository {
	return &userRepository{db: db}
}

func (r *userRepository) Create(user *models.User) error {
	return r.db.Create(user).Error
}

func (r *userRepository) GetByID(id uuid.UUID) (*models.User, error) {
	var user models.User
	if err := r.db.First(&user, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *userRepository) GetByTelegramUserID(telegramUserID int64) (*models.User, error) {
	var user models.User
	if err := r.db.Where("telegram_user_id = ?", telegramUserID).First(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *userRepository) GetOrCreateByTelegramUserID(telegramUserID int64, username *string) (*models.User, error) {
	user, err := r.GetByTelegramUserID(telegramUserID)
	if err == nil {
		// Update username if provided and different
		if username != nil && (user.Username == nil || *user.Username != *username) {
			user.Username = username
			if err := r.Update(user); err != nil {
				return nil, err
			}
		}
		return user, nil
	}

	if err != gorm.ErrRecordNotFound {
		return nil, err
	}

	// Create new user
	newUser := &models.User{
		TelegramUserID: telegramUserID,
		Username:       username,
	}
	if err := r.Create(newUser); err != nil {
		return nil, err
	}
	return newUser, nil
}

func (r *userRepository) Update(user *models.User) error {
	return r.db.Save(user).Error
}

func (r *userRepository) Delete(id uuid.UUID) error {
	return r.db.Delete(&models.User{}, "id = ?", id).Error
}
