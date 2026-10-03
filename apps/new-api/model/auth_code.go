package model

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"time"

	"gorm.io/gorm"
)

type AuthCode struct {
	Code        string    `gorm:"column:code;primary_key" json:"code"`
	UserId      int       `gorm:"column:user_id;not null;index:idx_user_expires,priority:1" json:"user_id"`
	RedirectUri string    `gorm:"column:redirect_uri;not null" json:"redirect_uri"`
	State       string    `gorm:"column:state" json:"state"`
	Used        bool      `gorm:"column:used;default:false" json:"used"`
	ExpiresAt   time.Time `gorm:"column:expires_at;not null;index:idx_user_expires,priority:2" json:"expires_at"`
	CreatedAt   time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
}

func (AuthCode) TableName() string {
	return "auth_codes"
}

func GenerateAuthCode(userId int, redirectUri, state string) (*AuthCode, error) {
	bytes := make([]byte, 24)
	if _, err := rand.Read(bytes); err != nil {
		return nil, err
	}
	code := base64.RawURLEncoding.EncodeToString(bytes)

	authCode := &AuthCode{
		Code:        code,
		UserId:      userId,
		RedirectUri: redirectUri,
		State:       state,
		Used:        false,
		ExpiresAt:   time.Now().Add(5 * time.Minute),
	}
	err := DB.Create(authCode).Error
	return authCode, err
}

func ConsumeAuthCode(code, redirectUri string) (*AuthCode, error) {
	var authCode AuthCode
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("code = ? AND redirect_uri = ?", code, redirectUri).First(&authCode).Error; err != nil {
			return err
		}
		if authCode.Used {
			return errors.New("auth code already used")
		}
		if time.Now().After(authCode.ExpiresAt) {
			return errors.New("auth code expired")
		}
		return tx.Model(&authCode).Update("used", true).Error
	})
	return &authCode, err
}

func CleanupExpiredAuthCodes() error {
	return DB.Where("expires_at < ?", time.Now()).Delete(&AuthCode{}).Error
}
