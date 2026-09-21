package store

import (
	"gorm.io/gorm"
	"time"
)

// MFAConfig holds the encrypted TOTP credential for one user. Recovery codes
// are bcrypt hashes serialized as JSON; plaintext codes are never persisted.
type MFAConfig struct {
	UserID            string    `gorm:"primaryKey"`
	SecretEncrypted   string    `gorm:"column:secret_encrypted;not null"`
	RecoveryCodesJSON string    `gorm:"column:recovery_codes_json;not null"`
	Enabled           bool      `gorm:"not null;default:false"`
	EnabledAt         time.Time `gorm:"not null"`
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

func (MFAConfig) TableName() string { return "user_mfa" }

type MFAStore struct{ db *gorm.DB }

func (s *MFAStore) initTables() error { return s.db.AutoMigrate(&MFAConfig{}) }

func (s *MFAStore) Get(userID string) (*MFAConfig, error) {
	var item MFAConfig
	if err := s.db.Where("user_id = ?", userID).First(&item).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

func (s *MFAStore) Save(item *MFAConfig) error { return s.db.Save(item).Error }

func (s *MFAStore) Disable(userID string) error {
	return s.db.Where("user_id = ?", userID).Updates(map[string]interface{}{"enabled": false, "secret_encrypted": "", "recovery_codes_json": "", "enabled_at": time.Time{}}).Error
}
