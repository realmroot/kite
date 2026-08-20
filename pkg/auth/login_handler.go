package auth

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/zxh326/kite/pkg/common"
	"github.com/zxh326/kite/pkg/model"
	"k8s.io/klog/v2"
)

func (h *AuthHandler) Login(c *gin.Context) {
	if !oidcConfigured() {
		c.JSON(http.StatusServiceUnavailable, gin.H{"message": "OIDC is not configured"})
		return
	}
	authURL, err := h.oidc.authorizationURL(c)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"auth_url": authURL, "provider": common.OIDCProviderName})
}

func (h *AuthHandler) Callback(c *gin.Context) {
	base := common.Base
	code := c.Query("code")
	if code == "" {
		c.Redirect(http.StatusFound, base+"/login?error=callback_error&reason=missing_code")
		return
	}
	token, claims, err := h.oidc.exchange(c, code, c.Query("state"))
	if err != nil {
		klog.Warningf("OIDC callback failed: %v", err)
		c.Redirect(http.StatusFound, base+"/login?error=callback_error&reason=callback_failed")
		return
	}
	user := userFromOIDCClaims(claims)
	if err := model.FindWithSubOrUpsertUser(user); err != nil {
		c.Redirect(http.StatusFound, base+"/login?error=callback_error&reason=user_upsert_failed")
		return
	}
	provider, err := h.oidc.discoveredProvider(c.Request.Context())
	if err != nil {
		c.Redirect(http.StatusFound, base+"/login?error=callback_error&reason=provider_discovery_failed")
		return
	}
	rawIDToken, _ := token.Extra("id_token").(string)
	_, idToken, err := h.oidc.verifyIDToken(c.Request.Context(), provider, rawIDToken, "")
	if err != nil {
		klog.Warningf("Verify OIDC token for session creation: %v", err)
		c.Redirect(http.StatusFound, base+"/login?error=callback_error&reason=session_creation_failed")
		return
	}
	if err := createOIDCSession(c, user, token, idToken); err != nil {
		klog.Errorf("Create OIDC session: %v", err)
		c.Redirect(http.StatusFound, base+"/login?error=callback_error&reason=session_creation_failed")
		return
	}
	c.Redirect(http.StatusFound, base+"/")
}

func (h *AuthHandler) Logout(c *gin.Context) {
	if opaqueToken, err := c.Cookie(sessionCookieName); err == nil {
		if session, err := model.GetOIDCSessionByHash(hashOpaqueValue(opaqueToken)); err == nil {
			_ = model.DeleteOIDCSession(session)
		}
	}
	setCookieSecure(c, sessionCookieName, "", -1)
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Logged out successfully"})
}

func (h *AuthHandler) GetUser(c *gin.Context) {
	value, exists := c.Get("user")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Not authenticated"})
		return
	}
	currentUser := value.(model.User)
	setting, err := model.GetGeneralSetting()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to load general setting: %v", err)})
		return
	}
	globalPreference := strings.TrimSpace(setting.GlobalSidebarPreference)
	isAdmin := platformAdmin(currentUser)
	if globalPreference != "" && (!isAdmin || strings.TrimSpace(currentUser.SidebarPreference) == "") {
		currentUser.SidebarPreference = globalPreference
	}
	c.JSON(http.StatusOK, gin.H{
		"user": currentUser, "platformAdmin": isAdmin,
		"hasGlobalSidebarPreference": globalPreference != "",
		"globalSidebarPreference":    globalPreference,
	})
}

func (h *AuthHandler) RefreshToken(c *gin.Context) {
	if _, _, _, err := h.oidc.authenticatedSession(c); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Failed to refresh OIDC session"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Token refreshed successfully"})
}

func setCookieSecure(c *gin.Context, name, value string, maxAge int) {
	secure := strings.HasPrefix(common.Host, "https://")
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(name, value, maxAge, "/", "", secure, true)
}
