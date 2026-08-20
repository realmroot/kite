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

func DeleteOIDCSession(session *OIDCSession) error {
	return RevokeOIDCSession(session.ID, "OIDC session ended; re-enable this task to authorize it again")
}

func RevokeOIDCSession(sessionID uint, taskError string) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&ScheduledTask{}).
			Where("oidc_session_id = ? AND enabled = ?", sessionID, true).
			Updates(map[string]any{
				"enabled":     false,
				"next_run_at": nil,
				"last_error":  taskError,
			}).Error; err != nil {
			return err
		}
		return tx.Delete(&OIDCSession{}, sessionID).Error
	})
}

func DeleteInactiveOIDCSessions(lastUsedBefore time.Time) error {
	return DB.Where("updated_at <= ?", lastUsedBefore).
		Where("NOT EXISTS (?)", DB.Model(&ScheduledTask{}).
			Select("1").
			Where("scheduled_tasks.oidc_session_id = oidc_sessions.id AND scheduled_tasks.enabled = ?", true)).
		Delete(&OIDCSession{}).Error
}
