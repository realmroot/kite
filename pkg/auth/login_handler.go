package auth

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/zxh326/kite/pkg/common"
	"github.com/zxh326/kite/pkg/mfa"
	"github.com/zxh326/kite/pkg/model"
	"github.com/zxh326/kite/pkg/passkey"
	"github.com/zxh326/kite/pkg/rbac"
	"gorm.io/gorm"
	"k8s.io/klog/v2"
)

func (h *AuthHandler) Login(c *gin.Context) {
	if !realmrootConfigured() {
		c.JSON(http.StatusServiceUnavailable, gin.H{"message": "Realmroot OIDC is not configured"})
		return
	}
	authURL, err := h.oidc.authorizationURL(c)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"auth_url": authURL,
		"provider": "realmroot",
	})
}

func (h *AuthHandler) PasswordLogin(c *gin.Context) {
	setting, err := model.GetGeneralSetting()
	if err == nil && setting.PasswordLoginDisabled {
		c.JSON(http.StatusForbidden, gin.H{"error": "Password login is disabled"})
		return
	}
	h.handleCredentialLogin(c, model.AuthProviderPassword, h.authenticatePasswordUser)
}

func (h *AuthHandler) CreateSuperUser(c *gin.Context) {
	var userreq struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
		Name     string `json:"name"`
	}
	if err := c.ShouldBindJSON(&userreq); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	uc, err := model.CountUsers()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to count users"})
		return
	}

	if uc > 0 {
		c.JSON(http.StatusForbidden, gin.H{"error": "super user already exists"})
		return
	}
	user := &model.User{
		Username: userreq.Username,
		Password: userreq.Password,
		Name:     userreq.Name,
		Provider: "password",
	}

	if err := model.AddSuperUser(user); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create super user"})
		return
	}
	jwtToken, err := h.manager.GenerateJWT(user, "")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate JWT"})
		return
	}
	setCookieSecure(c, "auth_token", jwtToken, common.CookieExpirationSeconds)
	rbac.TriggerSync()
	c.JSON(http.StatusCreated, user)
}

func (h *AuthHandler) LDAPLogin(c *gin.Context) {
	h.handleCredentialLogin(c, model.AuthProviderLDAP, h.authenticateAndSyncLDAPUser)
}

func (h *AuthHandler) PasskeyLoginBegin(c *gin.Context) {
	enabled, err := passkey.Enabled()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load general setting"})
		return
	}
	if !enabled {
		c.JSON(http.StatusForbidden, gin.H{"error": "passkey login is disabled"})
		return
	}
	assertion, err := passkey.BeginLogin(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to start passkey login"})
		return
	}
	c.JSON(http.StatusOK, assertion)
}

func (h *AuthHandler) PasskeyLoginFinish(c *gin.Context) {
	enabled, err := passkey.Enabled()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load general setting"})
		return
	}
	if !enabled {
		c.JSON(http.StatusForbidden, gin.H{"error": "passkey login is disabled"})
		return
	}
	user, err := passkey.FinishLogin(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid passkey"})
		return
	}
	h.completePasswordLikeLogin(c, user)
}

func (h *AuthHandler) handleCredentialLogin(c *gin.Context, provider string, authenticate credentialAuthenticator) {
	var req common.PasswordLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request payload"})
		return
	}

	username := strings.TrimSpace(req.Username)
	if username == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request payload"})
		return
	}
	clientIP := c.ClientIP()
	if credentialLoginAttempts.isBlocked(clientIP) {
		c.JSON(http.StatusTooManyRequests, gin.H{"error": tooManyCredentialLoginAttemptsError})
		return
	}

	user, err := authenticate(username, req.Password)
	if err != nil {
		errMsg := fmt.Sprintf("%s login failed for %s: %v", strings.ToUpper(provider), username, err)
		if isCredentialFailure(err) {
			if shouldRecordCredentialLoginFailure(provider, err) && credentialLoginAttempts.recordFailure(clientIP) {
				c.JSON(http.StatusTooManyRequests, gin.H{"error": tooManyCredentialLoginAttemptsError})
				return
			}
			c.JSON(http.StatusUnauthorized, gin.H{"error": errMsg})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": errMsg})
		return
	}

	if provider == model.AuthProviderPassword && user.MFAEnabled {
		if strings.TrimSpace(req.MFACode) == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "mfa_required"})
			return
		}
		if !mfa.Verify(string(user.MFASecret), req.MFACode) {
			if credentialLoginAttempts.recordFailure(clientIP) {
				c.JSON(http.StatusTooManyRequests, gin.H{"error": tooManyCredentialLoginAttemptsError})
				return
			}
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid_mfa_code"})
			return
		}
	}

	h.completePasswordLikeLogin(c, user)
}

func shouldRecordCredentialLoginFailure(provider string, err error) bool {
	switch provider {
	case model.AuthProviderPassword:
		return errors.Is(err, errInvalidCredentials)
	case model.AuthProviderLDAP:
		return errors.Is(err, ErrLDAPInvalidCredentials)
	default:
		return false
	}
}

func (h *AuthHandler) completePasswordLikeLogin(c *gin.Context, user *model.User) {
	if !user.Enabled {
		c.JSON(http.StatusForbidden, gin.H{"error": "insufficient permissions"})
		return
	}

	roles := rbac.GetUserRoles(*user)
	if len(roles) == 0 {
		c.JSON(http.StatusForbidden, gin.H{"error": "insufficient permissions"})
		return
	}

	if err := model.LoginUser(user); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed login"})
		return
	}

	jwtToken, err := h.manager.GenerateJWT(user, "")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate JWT"})
		return
	}

	setCookieSecure(c, "auth_token", jwtToken, common.CookieExpirationSeconds)

	c.Status(http.StatusNoContent)
}

func (h *AuthHandler) authenticateAndSyncLDAPUser(username, password string) (*model.User, error) {
	setting, err := model.GetLDAPSetting()
	if err != nil {
		return nil, err
	}

	ldapUser, err := h.ldap.Authenticate(setting, username, password)
	if err != nil {
		return nil, err
	}

	syncedUser, err := model.UpsertLDAPUser(ldapUser)
	if err != nil {
		if errors.Is(err, model.ErrUserProviderConflict) {
			return nil, ErrLDAPInvalidCredentials
		}
		return nil, err
	}

	return syncedUser, nil
}

func (h *AuthHandler) authenticatePasswordUser(username, password string) (*model.User, error) {
	user, err := model.GetUserByUsername(username)
	switch {
	case err == nil:
		if user.Provider != "" && user.Provider != model.AuthProviderPassword {
			return nil, errInvalidCredentials
		}
		if !model.CheckPassword(user.Password, password) {
			return nil, errInvalidCredentials
		}
		return user, nil
	case errors.Is(err, gorm.ErrRecordNotFound):
		return nil, errInvalidCredentials
	default:
		return nil, err
	}
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
		klog.Warningf("Realmroot OIDC callback failed: %v", err)
		c.Redirect(http.StatusFound, base+"/login?error=callback_error&reason=callback_failed")
		return
	}
	user := userFromRealmrootClaims(claims)
	if err := model.FindWithSubOrUpsertUser(user); err != nil {
		c.Redirect(http.StatusFound, base+"/login?error=callback_error&reason=user_upsert_failed")
		return
	}
	if !user.Enabled {
		c.Redirect(http.StatusFound, base+"/login?error=user_disabled&reason=user_disabled")
		return
	}
	provider, err := h.oidc.discoveredProvider(c.Request.Context())
	if err != nil {
		c.Redirect(http.StatusFound, base+"/login?error=callback_error&reason=provider_discovery_failed")
		return
	}
	rawIDToken, _ := token.Extra("id_token").(string)
	_, idToken, err := h.oidc.verifyIDToken(c.Request.Context(), provider, rawIDToken, "")
	if err != nil || createRealmrootSession(c, user, token, idToken) != nil {
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
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Logged out successfully",
	})
}

func (h *AuthHandler) GetUser(c *gin.Context) {
	user, exists := c.Get("user")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "Not authenticated",
		})
		return
	}

	currentUser := user.(model.User)
	isAdmin := platformAdmin(currentUser)
	setting, err := model.GetGeneralSetting()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("Failed to load general setting: %v", err),
		})
		return
	}

	globalSidebarPreference := strings.TrimSpace(setting.GlobalSidebarPreference)
	hasGlobalSidebarPreference := globalSidebarPreference != ""
	if hasGlobalSidebarPreference && (!isAdmin || strings.TrimSpace(currentUser.SidebarPreference) == "") {
		currentUser.SidebarPreference = globalSidebarPreference
	}

	c.JSON(http.StatusOK, gin.H{
		"user":                       currentUser,
		"hasGlobalSidebarPreference": hasGlobalSidebarPreference,
		"globalSidebarPreference":    globalSidebarPreference,
	})
}

func (h *AuthHandler) RefreshToken(c *gin.Context) {
	_, _, err := h.oidc.authenticatedSession(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "Failed to refresh Realmroot session",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Token refreshed successfully",
	})
}

func setCookieSecure(c *gin.Context, name, value string, maxAge int) {
	secure := strings.HasPrefix(common.Host, "https://") || (c.Request != nil && (c.Request.TLS != nil || strings.EqualFold(c.Request.Header.Get("X-Forwarded-Proto"), "https")))

	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(name, value, maxAge+60*60, "/", "", secure, true)
}
