package model

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"time"

	expirable "github.com/hashicorp/golang-lru/v2/expirable"
	"gorm.io/gorm"
)

// User is a local presentation profile for a verified OIDC principal. Identity
// and account security remain owned by the issuer; Kite persists no password,
// role, API key, MFA, passkey, or enable/disable state.
type User struct {
	Model
	Issuer      string      `json:"issuer" gorm:"type:varchar(500);not null;uniqueIndex:idx_users_oidc_principal,priority:1"`
	Sub         string      `json:"sub" gorm:"type:varchar(500);not null;uniqueIndex:idx_users_oidc_principal,priority:2"`
	Username    string      `json:"username" gorm:"type:varchar(255);not null;index"`
	Name        string      `json:"name,omitempty" gorm:"type:varchar(255)"`
	AvatarURL   string      `json:"avatar_url,omitempty" gorm:"type:text"`
	OIDCGroups  SliceString `json:"oidc_groups,omitempty" gorm:"column:oidc_groups;type:text"`
	LastLoginAt *time.Time  `json:"lastLoginAt,omitempty" gorm:"type:timestamp;index"`

	SidebarPreference string `json:"sidebar_preference,omitempty" gorm:"type:text"`
}

func (u *User) PrincipalKey() string {
	principal := fmt.Sprintf("%d:%s:%d:%s", len(u.Issuer), u.Issuer, len(u.Sub), u.Sub)
	return fmt.Sprintf("%x", sha256.Sum256([]byte(principal)))
}

var userCache = expirable.NewLRU[uint64, *User](256, nil, 30*time.Second)

func GetUserByIDCached(id uint64) (*User, error) {
	if cached, ok := userCache.Get(id); ok {
		copy := *cached
		return &copy, nil
	}
	user, err := GetUserByID(id)
	if err != nil {
		return nil, err
	}
	userCache.Add(id, user)
	copy := *user
	return &copy, nil
}

func InvalidateUserCache(id uint64) {
	userCache.Remove(id)
}

func GetUserByID(id uint64) (*User, error) {
	var user User
	if err := DB.First(&user, id).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

func FindWithSubOrUpsertUser(user *User) error {
	if user.Issuer == "" || user.Sub == "" {
		return errors.New("OIDC issuer and subject are required")
	}
	var existing User
	now := time.Now()
	user.LastLoginAt = &now
	err := DB.Where("issuer = ? AND sub = ?", user.Issuer, user.Sub).First(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return DB.Create(user).Error
	}
	if err != nil {
		return err
	}
	user.ID = existing.ID
	user.CreatedAt = existing.CreatedAt
	user.SidebarPreference = existing.SidebarPreference
	if err := DB.Save(user).Error; err != nil {
		return err
	}
	InvalidateUserCache(uint64(user.ID))
	return nil
}

func UpdateUser(user *User) error {
	if err := DB.Model(&User{}).Where("id = ?", user.ID).Update("sidebar_preference", user.SidebarPreference).Error; err != nil {
		return err
	}
	InvalidateUserCache(uint64(user.ID))
	return nil
}
