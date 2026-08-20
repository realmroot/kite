package model

import "time"

type DPoPProof struct {
	ID            uint      `gorm:"primarykey"`
	KeyThumbprint string    `gorm:"type:varchar(64);not null;uniqueIndex:idx_dpop_proofs_key_jti,priority:1"`
	JTI           string    `gorm:"type:varchar(255);not null;uniqueIndex:idx_dpop_proofs_key_jti,priority:2"`
	ExpiresAt     time.Time `gorm:"not null;index"`
	CreatedAt     time.Time `gorm:"not null"`
}

type ResourceAccessAudit struct {
	ID                uint      `json:"id" gorm:"primarykey"`
	CreatedAt         time.Time `json:"createdAt" gorm:"not null;index"`
	RequestID         string    `json:"requestId" gorm:"type:varchar(255);index"`
	ControllerSubject string    `json:"controllerSubject" gorm:"type:varchar(255);not null;index"`
	AgentIssuer       string    `json:"agentIssuer" gorm:"type:text;not null"`
	AgentSubject      string    `json:"agentSubject" gorm:"type:varchar(255);not null;index"`
	ClientID          string    `json:"clientId" gorm:"type:varchar(255);not null"`
	Scopes            string    `json:"scopes" gorm:"type:text;not null"`
	ClusterName       string    `json:"clusterName" gorm:"type:varchar(100);index"`
	Method            string    `json:"method" gorm:"type:varchar(10);not null"`
	Path              string    `json:"path" gorm:"type:text;not null"`
	Status            int       `json:"status" gorm:"not null"`
}
