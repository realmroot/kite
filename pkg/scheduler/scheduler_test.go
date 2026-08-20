package scheduler

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/zxh326/kite/pkg/model"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestPermanentTaskFailureDisablesTask(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.ScheduledTask{}); err != nil {
		t.Fatal(err)
	}
	previousDB := model.DB
	model.DB = db
	t.Cleanup(func() { model.DB = previousDB })
	next := time.Now().Add(time.Minute)
	task := model.ScheduledTask{
		ClusterName:     "kind",
		Type:            "test",
		Key:             "task",
		Enabled:         true,
		ScheduleType:    model.ScheduledTaskScheduleTypeInterval,
		IntervalMinutes: 60,
		NextRunAt:       &next,
	}
	if err := db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}

	NewManager().finish(task, time.Now(), &permanentTaskError{err: errors.New("grant revoked")})
	var updated model.ScheduledTask
	if err := db.First(&updated, task.ID).Error; err != nil {
		t.Fatal(err)
	}
	if updated.Enabled || updated.NextRunAt != nil || updated.LastError != "grant revoked" {
		t.Fatalf("updated task = %#v", updated)
	}
}

func TestUnboundAutoUpgradeCredentialFailsPermanently(t *testing.T) {
	executor := &helmReleaseAutoUpgradeExecutor{}
	err := executor.Run(context.Background(), model.ScheduledTask{
		CreatorID: 1,
		Payload:   `{"namespace":"default","resourceName":"demo"}`,
	})
	var permanent permanentError
	if !errors.As(err, &permanent) || !permanent.IsPermanent() {
		t.Fatalf("error = %#v, want permanent error", err)
	}
}

func TestAutoUpgradeRejectsMismatchedSessionIdentity(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.OIDCSession{}); err != nil {
		t.Fatal(err)
	}
	previousDB := model.DB
	model.DB = db
	t.Cleanup(func() { model.DB = previousDB })
	session := model.OIDCSession{TokenHash: "session", UserID: 2, IDToken: "token", ExpiresAt: time.Now().Add(time.Hour)}
	if err := db.Create(&session).Error; err != nil {
		t.Fatal(err)
	}

	err = (&helmReleaseAutoUpgradeExecutor{}).Run(context.Background(), model.ScheduledTask{
		CreatorID: 1, OIDCSessionID: session.ID,
		Payload: `{"namespace":"default","resourceName":"demo"}`,
	})
	var permanent permanentError
	if !errors.As(err, &permanent) || !permanent.IsPermanent() {
		t.Fatalf("error = %#v, want permanent identity mismatch", err)
	}
}

func TestAutoUpgradeTreatsMissingSessionAsPermanent(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.OIDCSession{}); err != nil {
		t.Fatal(err)
	}
	previousDB := model.DB
	model.DB = db
	t.Cleanup(func() { model.DB = previousDB })

	err = (&helmReleaseAutoUpgradeExecutor{}).Run(context.Background(), model.ScheduledTask{
		CreatorID: 1, OIDCSessionID: 42,
		Payload: `{"namespace":"default","resourceName":"demo"}`,
	})
	var permanent permanentError
	if !errors.As(err, &permanent) || !permanent.IsPermanent() {
		t.Fatalf("error = %#v, want permanent missing-session error", err)
	}
}

func TestRevokingOIDCSessionDisablesBoundTask(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.OIDCSession{}, &model.ScheduledTask{}); err != nil {
		t.Fatal(err)
	}
	previousDB := model.DB
	model.DB = db
	t.Cleanup(func() { model.DB = previousDB })
	session := model.OIDCSession{TokenHash: "session", UserID: 1, IDToken: "token", ExpiresAt: time.Now().Add(time.Hour)}
	if err := db.Create(&session).Error; err != nil {
		t.Fatal(err)
	}
	next := time.Now().Add(time.Minute)
	task := model.ScheduledTask{
		ClusterName: "kind", Type: HelmReleaseAutoUpgradeTaskType, Key: "default/demo",
		OIDCSessionID: session.ID, Enabled: true, ScheduleType: model.ScheduledTaskScheduleTypeInterval,
		IntervalMinutes: 60, NextRunAt: &next,
	}
	if err := db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}

	if err := model.RevokeOIDCSession(session.ID, "authorization revoked"); err != nil {
		t.Fatal(err)
	}
	var updated model.ScheduledTask
	if err := db.First(&updated, task.ID).Error; err != nil {
		t.Fatal(err)
	}
	if updated.Enabled || updated.NextRunAt != nil || updated.LastError != "authorization revoked" {
		t.Fatalf("task = %#v", updated)
	}
}
