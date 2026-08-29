package model

import (
	"time"

	"gorm.io/gorm"
)

// OIDCSession is the server-side half of Kite's BFF session. The browser only
// receives the opaque session token; provider tokens never leave the backend.
type OIDCSession struct {
	Model
	TokenHash    string       `json:"-" gorm:"type:varchar(64);uniqueIndex;not null"`
	UserID       uint         `json:"-" gorm:"index;not null"`
	IDToken      SecretString `json:"-" gorm:"type:text;not null"`
	AccessToken  SecretString `json:"-" gorm:"type:text"`
	RefreshToken SecretString `json:"-" gorm:"type:text"`
	ExpiresAt    time.Time    `json:"-" gorm:"index;not null"`
}

func (OIDCSession) TableName() string { return "oidc_sessions" }

func CreateOIDCSession(session *OIDCSession) error {
	return DB.Create(session).Error
}

func GetOIDCSessionByHash(tokenHash string) (*OIDCSession, error) {
	var session OIDCSession
	if err := DB.Where("token_hash = ?", tokenHash).First(&session).Error; err != nil {
		return nil, err
	}
	return &session, nil
}

func GetOIDCSessionByID(db *gorm.DB, id uint) (*OIDCSession, error) {
	var session OIDCSession
	if err := db.First(&session, id).Error; err != nil {
		return nil, err
	}
	return &session, nil
}

func UpdateOIDCSession(db *gorm.DB, session *OIDCSession, updates map[string]any) error {
	return db.Model(session).Updates(updates).Error
}

func TouchOIDCSession(sessionID uint) error {
	result := DB.Model(&OIDCSession{}).
		Where("id = ?", sessionID).
		UpdateColumn("updated_at", time.Now())
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func DeleteOIDCSession(session *OIDCSession) error {
	return RevokeOIDCSession(session.ID)
}

func RevokeOIDCSession(sessionID uint) error {
	return DB.Delete(&OIDCSession{}, sessionID).Error
}

func DeleteInactiveOIDCSessions(lastUsedBefore time.Time) error {
	return DB.Where("updated_at <= ?", lastUsedBefore).
		Delete(&OIDCSession{}).Error
}
