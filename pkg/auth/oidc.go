package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
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
	idTokenContextKey = "oidc-id-token"
)

type oidcClaims struct {
	Subject  string
	Username string
	Name     string
	Picture  string
	Groups   []string
}

type oidcAuthenticator struct {
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

func (a *oidcAuthenticator) discoveredProvider(ctx context.Context) (*oidc.Provider, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.provider != nil {
		return a.provider, nil
	}
	provider, err := oidc.NewProvider(ctx, common.OIDCIssuer)
	if err != nil {
		return nil, fmt.Errorf("discover OIDC provider: %w", err)
	}
	a.provider = provider
	return provider, nil
}

func oidcConfigured() bool {
	return common.OIDCIssuer != "" && common.OIDCClientID != "" && common.OIDCClientSecret != ""
}

func oidcRedirectURL(c *gin.Context) string {
	return strings.TrimRight(getRequestHost(c), "/") + common.Base + "/api/auth/callback"
}

func oidcOAuthConfig(c *gin.Context, provider *oidc.Provider) oauth2.Config {
	return oauth2.Config{
		ClientID:     common.OIDCClientID,
		ClientSecret: common.OIDCClientSecret,
		Endpoint:     provider.Endpoint(),
		RedirectURL:  oidcRedirectURL(c),
		Scopes:       append([]string(nil), common.OIDCScopes...),
	}
}

func (a *oidcAuthenticator) authorizationURL(c *gin.Context) (string, error) {
	provider, err := a.discoveredProvider(c.Request.Context())
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
	config := oidcOAuthConfig(c, provider)
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

func (a *oidcAuthenticator) exchange(c *gin.Context, code, state string) (*oauth2.Token, *oidcClaims, error) {
	expectedState, stateErr := c.Cookie("oauth_state")
	nonce, nonceErr := c.Cookie("oauth_nonce")
	verifier, verifierErr := c.Cookie("oauth_pkce")
	clearLoginCookies(c)
	if stateErr != nil || nonceErr != nil || verifierErr != nil || state == "" || state != expectedState {
		return nil, nil, errors.New("invalid OIDC callback state")
	}
	provider, err := a.discoveredProvider(c.Request.Context())
	if err != nil {
		return nil, nil, err
	}
	config := oidcOAuthConfig(c, provider)
	token, err := config.Exchange(c.Request.Context(), code, oauth2.VerifierOption(verifier))
	if err != nil {
		return nil, nil, fmt.Errorf("exchange authorization code: %w", err)
	}
	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok || rawIDToken == "" {
		return nil, nil, errors.New("OIDC provider did not return an ID token")
	}
	claims, _, err := a.verifyIDToken(c.Request.Context(), provider, rawIDToken, nonce)
	if err != nil {
		return nil, nil, err
	}
	return token, claims, nil
}

func (a *oidcAuthenticator) verifyIDToken(ctx context.Context, provider *oidc.Provider, rawIDToken, nonce string) (*oidcClaims, *oidc.IDToken, error) {
	idToken, err := provider.Verifier(&oidc.Config{ClientID: common.OIDCClientID}).Verify(ctx, rawIDToken)
	if err != nil {
		return nil, nil, fmt.Errorf("verify OIDC ID token: %w", err)
	}
	if nonce != "" && idToken.Nonce != nonce {
		return nil, nil, errors.New("OIDC nonce mismatch")
	}
	claims, err := configuredOIDCClaims(idToken)
	if err != nil {
		return nil, nil, err
	}
	return claims, idToken, nil
}

func configuredOIDCClaims(idToken *oidc.IDToken) (*oidcClaims, error) {
	var raw map[string]json.RawMessage
	if err := idToken.Claims(&raw); err != nil {
		return nil, fmt.Errorf("decode OIDC ID token claims: %w", err)
	}
	return configuredOIDCClaimsFromRaw(raw)
}

func configuredOIDCClaimsFromRaw(raw map[string]json.RawMessage) (*oidcClaims, error) {
	subject, err := oidcStringClaim(raw, "sub")
	if err != nil {
		return nil, err
	}
	if subject == "" {
		return nil, errors.New("OIDC ID token has no subject")
	}
	username, err := oidcStringClaim(raw, common.OIDCUsernameClaim)
	if err != nil {
		return nil, err
	}
	name, err := oidcStringClaim(raw, common.OIDCNameClaim)
	if err != nil {
		return nil, err
	}
	picture, err := oidcStringClaim(raw, common.OIDCPictureClaim)
	if err != nil {
		return nil, err
	}
	groups, err := oidcStringListClaim(raw, common.OIDCGroupsClaim)
	if err != nil {
		return nil, err
	}
	return &oidcClaims{
		Subject:  subject,
		Username: username,
		Name:     name,
		Picture:  picture,
		Groups:   groups,
	}, nil
}

func oidcStringClaim(raw map[string]json.RawMessage, name string) (string, error) {
	if name == "" || len(raw[name]) == 0 {
		return "", nil
	}
	var value string
	if err := json.Unmarshal(raw[name], &value); err != nil {
		return "", fmt.Errorf("OIDC claim %q must be a string: %w", name, err)
	}
	return value, nil
}

func oidcStringListClaim(raw map[string]json.RawMessage, name string) ([]string, error) {
	if name == "" || len(raw[name]) == 0 {
		return nil, nil
	}
	var values []string
	if err := json.Unmarshal(raw[name], &values); err == nil {
		return values, nil
	}
	value, err := oidcStringClaim(raw, name)
	if err != nil {
		return nil, fmt.Errorf("OIDC claim %q must be a string or string array: %w", name, err)
	}
	return []string{value}, nil
}

func userFromOIDCClaims(claims *oidcClaims) *model.User {
	username := claims.Username
	if username == "" {
		username = claims.Subject
	}
	return &model.User{
		Username:   username,
		Name:       claims.Name,
		AvatarURL:  claims.Picture,
		Provider:   "oidc",
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

func createOIDCSession(c *gin.Context, user *model.User, token *oauth2.Token, idToken *oidc.IDToken) error {
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
		for _, allowed := range common.PlatformAdminGroups {
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

func (a *oidcAuthenticator) authenticatedSession(c *gin.Context) (*model.User, string, error) {
	opaqueToken, err := c.Cookie(sessionCookieName)
	if err != nil || opaqueToken == "" {
		return nil, "", errors.New("session cookie is missing")
	}
	session, err := model.GetOIDCSessionByHash(hashOpaqueValue(opaqueToken))
	if err != nil {
		return nil, "", errors.New("session not found")
	}
	if time.Until(session.ExpiresAt) <= time.Minute {
		if err := a.refreshSession(c, session); err != nil {
			_ = model.DeleteOIDCSession(session)
			return nil, "", err
		}
	}
	user, err := model.GetUserByIDCached(uint64(session.UserID))
	if err != nil || !user.Enabled || user.Provider != "oidc" {
		_ = model.DeleteOIDCSession(session)
		return nil, "", errors.New("OIDC user not found")
	}
	return user, string(session.IDToken), nil
}

func (a *oidcAuthenticator) refreshSession(c *gin.Context, session *model.OIDCSession) error {
	if session.RefreshToken == "" {
		return errors.New("OIDC session cannot be refreshed")
	}
	provider, err := a.discoveredProvider(c.Request.Context())
	if err != nil {
		return err
	}
	config := oidcOAuthConfig(c, provider)
	current := &oauth2.Token{
		AccessToken:  string(session.AccessToken),
		RefreshToken: string(session.RefreshToken),
		Expiry:       time.Now().Add(-time.Minute),
	}
	refreshed, err := config.TokenSource(c.Request.Context(), current).Token()
	if err != nil {
		return fmt.Errorf("refresh OIDC token: %w", err)
	}
	rawIDToken, ok := refreshed.Extra("id_token").(string)
	if !ok || rawIDToken == "" {
		return errors.New("OIDC refresh did not return an ID token")
	}
	claims, idToken, err := a.verifyIDToken(c.Request.Context(), provider, rawIDToken, "")
	if err != nil {
		return err
	}
	user, err := model.GetUserByIDCached(uint64(session.UserID))
	if err != nil || user.Sub != claims.Subject {
		return errors.New("OIDC subject changed during refresh")
	}
	updatedUser := userFromOIDCClaims(claims)
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
