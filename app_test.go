package main

import (
	"testing"

	"github.com/zxh326/kite/pkg/common"
)

func TestValidateOIDCConfiguration(t *testing.T) {
	previousIssuer := common.OIDCIssuer
	previousClientID := common.OIDCClientID
	previousClientSecret := common.OIDCClientSecret
	previousScopes := common.OIDCScopes
	previousUsernameClaim := common.OIDCUsernameClaim
	previousGroupsClaim := common.OIDCGroupsClaim
	previousAdminGroups := common.PlatformAdminGroups
	t.Cleanup(func() {
		common.OIDCIssuer = previousIssuer
		common.OIDCClientID = previousClientID
		common.OIDCClientSecret = previousClientSecret
		common.OIDCScopes = previousScopes
		common.OIDCUsernameClaim = previousUsernameClaim
		common.OIDCGroupsClaim = previousGroupsClaim
		common.PlatformAdminGroups = previousAdminGroups
	})

	common.OIDCIssuer = "https://identity.example.com"
	common.OIDCClientID = "kite"
	common.OIDCClientSecret = "secret"
	common.OIDCScopes = []string{"openid", "profile"}
	common.OIDCUsernameClaim = "email"
	common.OIDCGroupsClaim = "groups"
	common.PlatformAdminGroups = []string{"platform-admins"}
	if err := validateOIDCConfiguration(); err != nil {
		t.Fatal(err)
	}

	common.OIDCScopes = []string{"profile"}
	if err := validateOIDCConfiguration(); err == nil {
		t.Fatal("configuration without openid scope was accepted")
	}
}
