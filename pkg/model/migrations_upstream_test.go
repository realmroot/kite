package model

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/realmroot/lightkite/pkg/common"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestRemoveLegacyIdentityAcceptsUpstreamSQLiteIndexesAndAcronymColumn(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`CREATE TABLE users (
			id integer primary key, created_at datetime, updated_at datetime,
			username varchar(50) not null, password varchar(255), name varchar(100),
			avatar_url text, provider varchar(50), o_id_c_groups text, last_login_at datetime,
			enabled boolean, sub varchar(255), mfa_enabled boolean, mfa_secret text,
			api_key text, sidebar_preference text
		)`,
		`CREATE INDEX idx_users_provider ON users(provider)`,
		`CREATE UNIQUE INDEX idx_users_username ON users(username)`,
		`CREATE TABLE oauth_providers (id integer primary key, name text, issuer text)`,
		`INSERT INTO oauth_providers VALUES (1, 'oidc', 'https://issuer.example')`,
		`INSERT INTO users (id, username, provider, sub, o_id_c_groups) VALUES (1, 'alice', 'oidc', 'alice-sub', '["operators"]')`,
	} {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatal(err)
		}
	}
	previousIssuer := common.OIDCIssuer
	common.OIDCIssuer = "https://issuer.example"
	t.Cleanup(func() { common.OIDCIssuer = previousIssuer })

	if err := runSchemaMigrations(db); err != nil {
		t.Fatal(err)
	}
	for _, column := range []string{"provider", "password", "enabled", "o_id_c_groups"} {
		if db.Migrator().HasColumn("users", column) {
			t.Fatalf("legacy column %s still exists", column)
		}
	}
	if !db.Migrator().HasColumn("users", "oidc_groups") {
		t.Fatal("canonical oidc_groups column is missing")
	}
	var user struct {
		Issuer string
		Sub    string
		Groups string `gorm:"column:oidc_groups"`
	}
	if err := db.Table("users").Select("issuer", "sub", "oidc_groups").First(&user).Error; err != nil {
		t.Fatal(err)
	}
	if user.Issuer != "https://issuer.example" || user.Sub != "alice-sub" || user.Groups != "[]" {
		t.Fatalf("migrated user = %#v", user)
	}
}
