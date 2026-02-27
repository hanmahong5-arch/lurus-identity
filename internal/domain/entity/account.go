// Package entity defines domain entities for lurus-identity.
package entity

import (
	"fmt"
	"time"
)

// Account is the unified Lurus ID for every user across all products.
type Account struct {
	ID            int64      `json:"id"             gorm:"primaryKey;autoIncrement"`
	LurusID       string     `json:"lurus_id"       gorm:"type:varchar(16);uniqueIndex;not null"`
	ZitadelSub    string     `json:"zitadel_sub"    gorm:"type:varchar(128);uniqueIndex"`
	DisplayName   string     `json:"display_name"   gorm:"type:varchar(64);not null"`
	AvatarURL     string     `json:"avatar_url"     gorm:"type:text"`
	Email         string     `json:"email"          gorm:"type:varchar(255);uniqueIndex;not null"`
	EmailVerified bool       `json:"email_verified" gorm:"default:false"`
	Phone         string     `json:"phone"          gorm:"type:varchar(32)"`
	PhoneVerified bool       `json:"phone_verified" gorm:"default:false"`
	Status        int16      `json:"status"         gorm:"default:1"` // 1=active 2=suspended 3=deleted
	Locale        string     `json:"locale"         gorm:"type:varchar(8);default:'zh-CN'"`
	ReferrerID    *int64     `json:"referrer_id"    gorm:"index"`
	AffCode       string     `json:"aff_code"       gorm:"type:varchar(32);uniqueIndex;not null"`
	CreatedAt     time.Time  `json:"created_at"     gorm:"autoCreateTime"`
	UpdatedAt     time.Time  `json:"updated_at"     gorm:"autoUpdateTime"`
}

func (Account) TableName() string { return "identity.accounts" }

// AccountStatus constants.
const (
	AccountStatusActive    int16 = 1
	AccountStatusSuspended int16 = 2
	AccountStatusDeleted   int16 = 3
)

// IsActive reports whether the account can authenticate.
func (a *Account) IsActive() bool { return a.Status == AccountStatusActive }

// GenerateLurusID produces a human-readable "LU" + zero-padded ID string.
func GenerateLurusID(id int64) string {
	return fmt.Sprintf("LU%07d", id)
}

// OAuthBinding records a third-party OAuth provider linkage.
type OAuthBinding struct {
	ID            int64     `json:"id"             gorm:"primaryKey;autoIncrement"`
	AccountID     int64     `json:"account_id"     gorm:"not null;index"`
	Provider      string    `json:"provider"       gorm:"type:varchar(32);not null"` // github/discord/wechat/telegram/linuxdo/oidc
	ProviderID    string    `json:"provider_id"    gorm:"type:varchar(128);not null"`
	ProviderEmail string    `json:"provider_email" gorm:"type:varchar(255)"`
	CreatedAt     time.Time `json:"created_at"     gorm:"autoCreateTime"`
}

func (OAuthBinding) TableName() string { return "identity.account_oauth_bindings" }
