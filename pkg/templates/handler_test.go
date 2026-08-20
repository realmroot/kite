package templates

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/zxh326/kite/pkg/model"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestInitTemplatesIsTransactionalAndIdempotent(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.ResourceTemplate{}); err != nil {
		t.Fatal(err)
	}
	previousDB := model.DB
	model.DB = db
	t.Cleanup(func() { model.DB = previousDB })

	if err := InitTemplates(); err != nil {
		t.Fatalf("first InitTemplates() error = %v", err)
	}
	var firstCount int64
	if err := db.Model(&model.ResourceTemplate{}).Count(&firstCount).Error; err != nil {
		t.Fatal(err)
	}
	if firstCount == 0 {
		t.Fatal("InitTemplates() created no built-in templates")
	}

	if err := InitTemplates(); err != nil {
		t.Fatalf("second InitTemplates() error = %v", err)
	}
	var secondCount int64
	if err := db.Model(&model.ResourceTemplate{}).Count(&secondCount).Error; err != nil {
		t.Fatal(err)
	}
	if secondCount != firstCount {
		t.Fatalf("template count changed from %d to %d", firstCount, secondCount)
	}
}

func TestInitTemplatesReturnsDatabaseFailure(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatal(err)
	}
	previousDB := model.DB
	model.DB = db
	t.Cleanup(func() { model.DB = previousDB })

	if err := InitTemplates(); err == nil {
		t.Fatal("InitTemplates() ignored a database failure")
	}
}
