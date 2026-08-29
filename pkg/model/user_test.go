package model

import (
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestFindWithSubOrUpsertUserDBUsesProvidedTransaction(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&User{}); err != nil {
		t.Fatal(err)
	}

	user := &User{Issuer: "https://issuer.example", Sub: "subject", Username: "before"}
	if err := db.Transaction(func(tx *gorm.DB) error {
		return FindWithSubOrUpsertUserDB(tx, user)
	}); err != nil {
		t.Fatal(err)
	}

	updated := &User{Issuer: user.Issuer, Sub: user.Sub, Username: "after"}
	if err := db.Transaction(func(tx *gorm.DB) error {
		return FindWithSubOrUpsertUserDB(tx, updated)
	}); err != nil {
		t.Fatal(err)
	}
	var stored User
	if err := db.Where("issuer = ? AND sub = ?", user.Issuer, user.Sub).First(&stored).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Username != "after" || stored.ID != user.ID {
		t.Fatalf("stored user = %#v, want updated user ID %d", stored, user.ID)
	}
}
