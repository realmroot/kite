package auth

import (
	"encoding/json"
	"testing"

	"github.com/zxh326/kite/pkg/common"
	"github.com/zxh326/kite/pkg/model"
)

func TestUserFromOIDCClaimsPreservesConfiguredIdentity(t *testing.T) {
	user := userFromOIDCClaims(&oidcClaims{
		Subject:  "user-123",
		Username: "alice",
		Name:     "Alice",
		Groups:   []string{"platform-admins", "developers"},
	})
	if user.Provider != "oidc" || user.Sub != "user-123" || user.Username != "alice" {
		t.Fatalf("user = %#v", user)
	}
	if len(user.OIDCGroups) != 2 || user.OIDCGroups[1] != "developers" {
		t.Fatalf("groups = %#v", user.OIDCGroups)
	}
}

func TestPlatformAdminComesOnlyFromConfiguredOIDCGroup(t *testing.T) {
	previous := common.PlatformAdminGroups
	common.PlatformAdminGroups = []string{"platform-admins"}
	t.Cleanup(func() { common.PlatformAdminGroups = previous })

	if !platformAdmin(model.User{OIDCGroups: []string{"platform-admins"}}) {
		t.Fatal("configured OIDC group should grant catalog administration")
	}
	if platformAdmin(model.User{OIDCGroups: []string{"developers"}}) {
		t.Fatal("unconfigured group must not grant catalog administration")
	}
}

func TestOIDCClaimDecodingSupportsProviderSpecificNames(t *testing.T) {
	raw := map[string]json.RawMessage{
		"roles":    json.RawMessage(`["operator","viewer"]`),
		"memberOf": json.RawMessage(`"engineering"`),
	}
	roles, err := oidcStringListClaim(raw, "roles")
	if err != nil || len(roles) != 2 || roles[0] != "operator" {
		t.Fatalf("roles = %#v, err = %v", roles, err)
	}
	groups, err := oidcStringListClaim(raw, "memberOf")
	if err != nil || len(groups) != 1 || groups[0] != "engineering" {
		t.Fatalf("groups = %#v, err = %v", groups, err)
	}
}

func TestConfiguredOIDCClaimsUseConfiguredTopLevelClaims(t *testing.T) {
	previousUsername := common.OIDCUsernameClaim
	previousGroups := common.OIDCGroupsClaim
	previousName := common.OIDCNameClaim
	previousPicture := common.OIDCPictureClaim
	common.OIDCUsernameClaim = "preferred_username"
	common.OIDCGroupsClaim = "roles"
	common.OIDCNameClaim = "display_name"
	common.OIDCPictureClaim = "avatar"
	t.Cleanup(func() {
		common.OIDCUsernameClaim = previousUsername
		common.OIDCGroupsClaim = previousGroups
		common.OIDCNameClaim = previousName
		common.OIDCPictureClaim = previousPicture
	})

	claims, err := configuredOIDCClaimsFromRaw(map[string]json.RawMessage{
		"sub":                json.RawMessage(`"subject-1"`),
		"preferred_username": json.RawMessage(`"alice"`),
		"roles":              json.RawMessage(`["developer","operator"]`),
		"display_name":       json.RawMessage(`"Alice"`),
		"avatar":             json.RawMessage(`"https://example.com/alice.png"`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if claims.Subject != "subject-1" || claims.Username != "alice" || claims.Name != "Alice" || claims.Picture == "" {
		t.Fatalf("claims = %#v", claims)
	}
	if len(claims.Groups) != 2 || claims.Groups[1] != "operator" {
		t.Fatalf("groups = %#v", claims.Groups)
	}
}

func TestConfiguredOIDCClaimsRequireStandardSubject(t *testing.T) {
	_, err := configuredOIDCClaimsFromRaw(map[string]json.RawMessage{})
	if err == nil {
		t.Fatal("missing sub claim was accepted")
	}
}
