package auth

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/zxh326/kite/pkg/common"
	"github.com/zxh326/kite/pkg/model"
)

type bootstrapSetupState struct {
	Initialized bool `json:"initialized"`
	Step        int  `json:"step"`
}

type bootstrapAuthOptions struct {
	Providers           []string `json:"providers"`
	CredentialProviders []string `json:"credentialProviders"`
	OAuthProviders      []string `json:"oauthProviders"`
	LoginPrompt         string   `json:"loginPrompt"`
	MFAEnabled          bool     `json:"mfaEnabled"`
	PasskeyLoginEnabled bool     `json:"passkeyLoginEnabled"`
}

type bootstrapCapabilities struct {
	AIEnabled      bool `json:"aiEnabled"`
	KubectlEnabled bool `json:"kubectlEnabled"`
}

type bootstrapResponse struct {
	Setup                      bootstrapSetupState   `json:"setup"`
	Auth                       bootstrapAuthOptions  `json:"auth"`
	Capabilities               bootstrapCapabilities `json:"capabilities"`
	User                       *model.User           `json:"user"`
	HasGlobalSidebarPreference bool                  `json:"hasGlobalSidebarPreference"`
	GlobalSidebarPreference    string                `json:"globalSidebarPreference"`
}

func (h *AuthHandler) Bootstrap(c *gin.Context) {
	setting, err := model.GetGeneralSetting()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load general setting"})
		return
	}

	setup := currentBootstrapSetup()
	var user *model.User
	if setup.Step == 0 && !setup.Initialized {
		c.SetCookie("auth_token", "", -1, "/", "", false, true)
	} else {
		user = h.bootstrapUser(c, setting)
	}

	globalSidebarPreference := strings.TrimSpace(setting.GlobalSidebarPreference)
	if user == nil {
		globalSidebarPreference = ""
	}

	c.JSON(http.StatusOK, bootstrapResponse{
		Setup: setup,
		Auth:  h.bootstrapAuth(),
		Capabilities: bootstrapCapabilities{
			AIEnabled:      false,
			KubectlEnabled: false,
		},
		User:                       user,
		HasGlobalSidebarPreference: globalSidebarPreference != "",
		GlobalSidebarPreference:    globalSidebarPreference,
	})
}

func currentBootstrapSetup() bootstrapSetupState {
	// Realmroot owns identity provisioning and clusters are managed after login.
	return bootstrapSetupState{Initialized: true, Step: 2}
}

func (h *AuthHandler) bootstrapAuth() bootstrapAuthOptions {
	return bootstrapAuthOptions{
		Providers:           []string{"realmroot"},
		CredentialProviders: []string{},
		OAuthProviders:      []string{"realmroot"},
		LoginPrompt:         "Sign in with Realmroot. Kubernetes permissions come directly from your Realmroot groups.",
		MFAEnabled:          false,
		PasskeyLoginEnabled: false,
	}
}

func (h *AuthHandler) bootstrapUser(c *gin.Context, setting *model.GeneralSetting) *model.User {
	user, _, err := h.oidc.authenticatedSession(c)
	if err != nil {
		return nil
	}

	currentUser := *user
	if platformAdmin(currentUser) {
		currentUser.Roles = []common.Role{platformAdminRole()}
	}
	applyBootstrapSidebarPreference(&currentUser, setting)

	return &currentUser
}

func applyBootstrapSidebarPreference(user *model.User, setting *model.GeneralSetting) {
	globalSidebarPreference := strings.TrimSpace(setting.GlobalSidebarPreference)
	if globalSidebarPreference != "" && (!platformAdmin(*user) || strings.TrimSpace(user.SidebarPreference) == "") {
		user.SidebarPreference = globalSidebarPreference
	}
}
