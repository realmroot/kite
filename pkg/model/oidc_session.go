package model

import "time"

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

func UpdateOIDCSession(session *OIDCSession, updates map[string]any) error {
	return DB.Model(session).Updates(updates).Error
}

func DeleteOIDCSession(session *OIDCSession) error {
	return DB.Delete(session).Error
}

func DeleteExpiredOIDCSessions(now time.Time) error {
	return DB.Where("expires_at <= ?", now).Delete(&OIDCSession{}).Error
}
