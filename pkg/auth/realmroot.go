package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/gin-gonic/gin"
	"github.com/zxh326/kite/pkg/common"
	"github.com/zxh326/kite/pkg/model"
	"golang.org/x/oauth2"
)

const (
	sessionCookieName = "kite_session"
	idTokenContextKey = "realmroot-id-token"
)

type realmrootClaims struct {
	Subject           string   `json:"sub"`
	Email             string   `json:"email"`
	Name              string   `json:"name"`
	Picture           string   `json:"picture"`
	PreferredUsername string   `json:"preferred_username"`
	Groups            []string `json:"groups"`
}

type realmrootOIDC struct {
	mu       sync.Mutex
	provider *oidc.Provider
}

func randomOpaqueValue() (string, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

func hashOpaqueValue(value string) string {
	hash := sha256.Sum256([]byte(value))
	return fmt.Sprintf("%x", hash[:])
}

func (r *realmrootOIDC) discoveredProvider(ctx context.Context) (*oidc.Provider, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.provider != nil {
		return r.provider, nil
	}
	provider, err := oidc.NewProvider(ctx, common.RealmrootIssuer)
	if err != nil {
		return nil, fmt.Errorf("discover Realmroot OIDC provider: %w", err)
	}
	r.provider = provider
	return provider, nil
}

func realmrootConfigured() bool {
	return common.RealmrootClientID != "" && common.RealmrootClientSecret != ""
}

func realmrootRedirectURL(c *gin.Context) string {
	return strings.TrimRight(getRequestHost(c), "/") + common.Base + "/api/auth/callback"
}

func realmrootOAuthConfig(c *gin.Context, provider *oidc.Provider) oauth2.Config {
	return oauth2.Config{
		ClientID:     common.RealmrootClientID,
		ClientSecret: common.RealmrootClientSecret,
		Endpoint:     provider.Endpoint(),
		RedirectURL:  realmrootRedirectURL(c),
		Scopes:       []string{oidc.ScopeOpenID, "profile", "email", "groups", "offline_access"},
	}
}

func (r *realmrootOIDC) authorizationURL(c *gin.Context) (string, error) {
	provider, err := r.discoveredProvider(c.Request.Context())
	if err != nil {
		return "", err
	}
	state, err := randomOpaqueValue()
	if err != nil {
		return "", err
	}
	nonce, err := randomOpaqueValue()
	if err != nil {
		return "", err
	}
	verifier, err := randomOpaqueValue()
	if err != nil {
		return "", err
	}
	setCookieSecure(c, "oauth_state", state, 600)
	setCookieSecure(c, "oauth_nonce", nonce, 600)
	setCookieSecure(c, "oauth_pkce", verifier, 600)
	config := realmrootOAuthConfig(c, provider)
	return config.AuthCodeURL(state,
		oauth2.AccessTypeOffline,
		oidc.Nonce(nonce),
		oauth2.S256ChallengeOption(verifier),
	), nil
}

func clearLoginCookies(c *gin.Context) {
	setCookieSecure(c, "oauth_state", "", -1)
	setCookieSecure(c, "oauth_nonce", "", -1)
	setCookieSecure(c, "oauth_pkce", "", -1)
}

func (r *realmrootOIDC) exchange(c *gin.Context, code, state string) (*oauth2.Token, *realmrootClaims, error) {
	expectedState, stateErr := c.Cookie("oauth_state")
	nonce, nonceErr := c.Cookie("oauth_nonce")
	verifier, verifierErr := c.Cookie("oauth_pkce")
	clearLoginCookies(c)
	if stateErr != nil || nonceErr != nil || verifierErr != nil || state == "" || state != expectedState {
		return nil, nil, errors.New("invalid OIDC callback state")
	}
	provider, err := r.discoveredProvider(c.Request.Context())
	if err != nil {
		return nil, nil, err
	}
	config := realmrootOAuthConfig(c, provider)
	token, err := config.Exchange(c.Request.Context(), code, oauth2.VerifierOption(verifier))
	if err != nil {
		return nil, nil, fmt.Errorf("exchange authorization code: %w", err)
	}
	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok || rawIDToken == "" {
		return nil, nil, errors.New("realmroot did not return an ID token")
	}
	claims, _, err := r.verifyIDToken(c.Request.Context(), provider, rawIDToken, nonce)
	if err != nil {
		return nil, nil, err
	}
	return token, claims, nil
}

func (r *realmrootOIDC) verifyIDToken(ctx context.Context, provider *oidc.Provider, rawIDToken, nonce string) (*realmrootClaims, *oidc.IDToken, error) {
	idToken, err := provider.Verifier(&oidc.Config{ClientID: common.RealmrootClientID}).Verify(ctx, rawIDToken)
	if err != nil {
		return nil, nil, fmt.Errorf("verify Realmroot ID token: %w", err)
	}
	if nonce != "" && idToken.Nonce != nonce {
		return nil, nil, errors.New("OIDC nonce mismatch")
	}
	var claims realmrootClaims
	if err := idToken.Claims(&claims); err != nil {
		return nil, nil, fmt.Errorf("decode Realmroot ID token claims: %w", err)
	}
	if claims.Subject == "" {
		return nil, nil, errors.New("realmroot ID token has no subject")
	}
	return &claims, idToken, nil
}

func userFromRealmrootClaims(claims *realmrootClaims) *model.User {
	username := claims.PreferredUsername
	if username == "" {
		username = claims.Email
	}
	if username == "" {
		username = claims.Subject
	}
	return &model.User{
		Username:   username,
		Name:       claims.Name,
		AvatarURL:  claims.Picture,
		Provider:   "realmroot",
		OIDCGroups: append(model.SliceString(nil), claims.Groups...),
		Sub:        claims.Subject,
		Enabled:    true,
	}
}

func sessionExpiry(token *oauth2.Token, idToken *oidc.IDToken) time.Time {
	expiresAt := idToken.Expiry
	if !token.Expiry.IsZero() && token.Expiry.Before(expiresAt) {
		expiresAt = token.Expiry
	}
	return expiresAt
}

func createRealmrootSession(c *gin.Context, user *model.User, token *oauth2.Token, idToken *oidc.IDToken) error {
	if err := model.DeleteExpiredOIDCSessions(time.Now()); err != nil {
		return fmt.Errorf("delete expired OIDC sessions: %w", err)
	}
	rawIDToken, _ := token.Extra("id_token").(string)
	opaqueToken, err := randomOpaqueValue()
	if err != nil {
		return err
	}
	session := &model.OIDCSession{
		TokenHash:    hashOpaqueValue(opaqueToken),
		UserID:       user.ID,
		IDToken:      model.SecretString(rawIDToken),
		AccessToken:  model.SecretString(token.AccessToken),
		RefreshToken: model.SecretString(token.RefreshToken),
		ExpiresAt:    sessionExpiry(token, idToken),
	}
	if err := model.CreateOIDCSession(session); err != nil {
		return err
	}
	setCookieSecure(c, sessionCookieName, opaqueToken, common.CookieExpirationSeconds)
	return nil
}

func platformAdmin(user model.User) bool {
	for _, actual := range user.OIDCGroups {
		for _, allowed := range common.RealmrootAdminGroups {
			if actual == allowed {
				return true
			}
		}
	}
	return false
}

func platformAdminRole() common.Role {
	return common.Role{
		Name:       "admin",
		Clusters:   []string{"*"},
		Resources:  []string{"*"},
		Namespaces: []string{"*"},
		Verbs:      []string{"*"},
	}
}

func (r *realmrootOIDC) authenticatedSession(c *gin.Context) (*model.User, string, error) {
	opaqueToken, err := c.Cookie(sessionCookieName)
	if err != nil || opaqueToken == "" {
		return nil, "", errors.New("session cookie is missing")
	}
	session, err := model.GetOIDCSessionByHash(hashOpaqueValue(opaqueToken))
	if err != nil {
		return nil, "", errors.New("session not found")
	}
	if time.Until(session.ExpiresAt) <= time.Minute {
		if err := r.refreshSession(c, session); err != nil {
			_ = model.DeleteOIDCSession(session)
			return nil, "", err
		}
	}
	user, err := model.GetUserByIDCached(uint64(session.UserID))
	if err != nil || !user.Enabled || user.Provider != "realmroot" {
		_ = model.DeleteOIDCSession(session)
		return nil, "", errors.New("realmroot user not found")
	}
	return user, string(session.IDToken), nil
}

func (r *realmrootOIDC) refreshSession(c *gin.Context, session *model.OIDCSession) error {
	if session.RefreshToken == "" {
		return errors.New("realmroot session cannot be refreshed")
	}
	provider, err := r.discoveredProvider(c.Request.Context())
	if err != nil {
		return err
	}
	config := realmrootOAuthConfig(c, provider)
	current := &oauth2.Token{
		AccessToken:  string(session.AccessToken),
		RefreshToken: string(session.RefreshToken),
		Expiry:       time.Now().Add(-time.Minute),
	}
	refreshed, err := config.TokenSource(c.Request.Context(), current).Token()
	if err != nil {
		return fmt.Errorf("refresh Realmroot token: %w", err)
	}
	rawIDToken, ok := refreshed.Extra("id_token").(string)
	if !ok || rawIDToken == "" {
		return errors.New("realmroot refresh did not return an ID token")
	}
	claims, idToken, err := r.verifyIDToken(c.Request.Context(), provider, rawIDToken, "")
	if err != nil {
		return err
	}
	user, err := model.GetUserByIDCached(uint64(session.UserID))
	if err != nil || user.Sub != claims.Subject {
		return errors.New("realmroot subject changed during refresh")
	}
	updatedUser := userFromRealmrootClaims(claims)
	updatedUser.ID = user.ID
	updatedUser.CreatedAt = user.CreatedAt
	updatedUser.SidebarPreference = user.SidebarPreference
	if err := model.FindWithSubOrUpsertUser(updatedUser); err != nil {
		return err
	}
	refreshToken := refreshed.RefreshToken
	if refreshToken == "" {
		refreshToken = string(session.RefreshToken)
	}
	expiresAt := sessionExpiry(refreshed, idToken)
	if err := model.UpdateOIDCSession(session, map[string]any{
		"id_token":      model.SecretString(rawIDToken),
		"access_token":  model.SecretString(refreshed.AccessToken),
		"refresh_token": model.SecretString(refreshToken),
		"expires_at":    expiresAt,
	}); err != nil {
		return err
	}
	session.IDToken = model.SecretString(rawIDToken)
	session.AccessToken = model.SecretString(refreshed.AccessToken)
	session.RefreshToken = model.SecretString(refreshToken)
	session.ExpiresAt = expiresAt
	return nil
}
