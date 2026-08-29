package auth

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/realmroot/lightkite/pkg/common"
	"github.com/realmroot/lightkite/pkg/model"
	"k8s.io/klog/v2"
)

type bootstrapAuthOptions struct {
	ProviderName string `json:"providerName"`
	LoginPrompt  string `json:"loginPrompt"`
}

type bootstrapCapabilities struct {
	KubectlEnabled bool `json:"kubectlEnabled"`
}

type bootstrapResponse struct {
	Auth                       bootstrapAuthOptions  `json:"auth"`
	Capabilities               bootstrapCapabilities `json:"capabilities"`
	User                       *model.User           `json:"user"`
	PlatformAdmin              bool                  `json:"platformAdmin"`
	HasGlobalSidebarPreference bool                  `json:"hasGlobalSidebarPreference"`
	GlobalSidebarPreference    string                `json:"globalSidebarPreference"`
}

func (h *AuthHandler) Bootstrap(c *gin.Context) {
	setting, err := model.GetGeneralSetting()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load general setting"})
		return
	}

	user, authErr := h.bootstrapUser(c, setting)
	if authErr != nil {
		klog.V(2).Infof("OIDC bootstrap authentication failed: %v", authErr)
		if transientSessionError(authErr) {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "OIDC session refresh is temporarily unavailable"})
			return
		}
	}

	globalSidebarPreference := strings.TrimSpace(setting.GlobalSidebarPreference)
	if user == nil {
		globalSidebarPreference = ""
	}

	c.JSON(http.StatusOK, bootstrapResponse{
		Auth: h.bootstrapAuth(setting),
		Capabilities: bootstrapCapabilities{
			KubectlEnabled: setting.KubectlEnabled,
		},
		User:                       user,
		PlatformAdmin:              user != nil && platformAdmin(*user),
		HasGlobalSidebarPreference: globalSidebarPreference != "",
		GlobalSidebarPreference:    globalSidebarPreference,
	})
}

func (h *AuthHandler) bootstrapAuth(setting *model.GeneralSetting) bootstrapAuthOptions {
	loginPrompt := strings.TrimSpace(setting.LoginPrompt)
	if loginPrompt == "" {
		loginPrompt = "Kubernetes permissions come directly from your identity provider claims."
	}
	return bootstrapAuthOptions{
		ProviderName: common.OIDCProviderName,
		LoginPrompt:  loginPrompt,
	}
}

func (h *AuthHandler) bootstrapUser(c *gin.Context, setting *model.GeneralSetting) (*model.User, error) {
	user, _, _, err := h.oidc.authenticatedSession(c)
	if err != nil {
		return nil, err
	}

	currentUser := *user
	applyBootstrapSidebarPreference(&currentUser, setting)

	return &currentUser, nil
}

func applyBootstrapSidebarPreference(user *model.User, setting *model.GeneralSetting) {
	globalSidebarPreference := strings.TrimSpace(setting.GlobalSidebarPreference)
	if globalSidebarPreference != "" && (!platformAdmin(*user) || strings.TrimSpace(user.SidebarPreference) == "") {
		user.SidebarPreference = globalSidebarPreference
	}
}
