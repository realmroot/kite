package auth

import (
	"testing"

	"github.com/zxh326/kite/pkg/common"
	"github.com/zxh326/kite/pkg/model"
)

func TestUserFromRealmrootClaimsPreservesGroups(t *testing.T) {
	user := userFromRealmrootClaims(&realmrootClaims{
		Subject:           "user-123",
		Email:             "alice@example.com",
		Name:              "Alice",
		PreferredUsername: "alice",
		Groups:            []string{"platform-admins", "developers"},
	})
	if user.Provider != "realmroot" || user.Sub != "user-123" || user.Username != "alice" {
		t.Fatalf("user = %#v", user)
	}
	if len(user.OIDCGroups) != 2 || user.OIDCGroups[1] != "developers" {
		t.Fatalf("groups = %#v", user.OIDCGroups)
	}
}

func TestPlatformAdminComesOnlyFromConfiguredRealmrootGroup(t *testing.T) {
	previous := common.RealmrootAdminGroups
	common.RealmrootAdminGroups = []string{"platform-admins"}
	t.Cleanup(func() { common.RealmrootAdminGroups = previous })

	if !platformAdmin(model.User{OIDCGroups: []string{"platform-admins"}}) {
		t.Fatal("configured Realmroot group should grant catalog administration")
	}
	if platformAdmin(model.User{OIDCGroups: []string{"developers"}}) {
		t.Fatal("unconfigured group must not grant catalog administration")
	}
}
