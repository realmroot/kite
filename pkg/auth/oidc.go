package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/gin-gonic/gin"
	"github.com/zxh326/kite/pkg/common"
	"github.com/zxh326/kite/pkg/model"
	"golang.org/x/oauth2"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	sessionCookieName     = "kite_session"
	idTokenContextKey     = "oidc-id-token"
	accessTokenContextKey = "oidc-access-token"
	sessionLockShards     = 64
	oidcRefreshTimeout    = 30 * time.Second
)

type oidcClaims struct {
	Subject  string
	Username string
	Name     string
	Picture  string
	Groups   []string
}

type oidcAuthenticator struct {
	mu                  sync.Mutex
	sessionRefreshLocks [sessionLockShards]sync.Mutex
	provider            *oidc.Provider
	clientOnce          sync.Once
	client              *http.Client
	clientErr           error
}

type SessionCredentialError struct {
	Err       error
	Permanent bool
}

func (e *SessionCredentialError) Error() string     { return e.Err.Error() }
func (e *SessionCredentialError) Unwrap() error     { return e.Err }
func (e *SessionCredentialError) IsPermanent() bool { return e.Permanent }

func (a *oidcAuthenticator) context(ctx context.Context) (context.Context, error) {
	if common.OIDCCAFile == "" {
		return ctx, nil
	}
	a.clientOnce.Do(func() {
		ca, err := os.ReadFile(common.OIDCCAFile)
		if err != nil {
			a.clientErr = fmt.Errorf("read OIDC CA file: %w", err)
			return
		}
		roots, err := x509.SystemCertPool()
		if err != nil {
			a.clientErr = fmt.Errorf("load system CA pool: %w", err)
			return
		}
		if !roots.AppendCertsFromPEM(ca) {
			a.clientErr = errors.New("OIDC CA file contains no valid PEM certificates")
			return
		}
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.TLSClientConfig = &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}
		a.client = &http.Client{Transport: transport, Timeout: 30 * time.Second}
	})
	if a.clientErr != nil {
		return nil, a.clientErr
	}
	return oidc.ClientContext(ctx, a.client), nil
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
	oidcContext, err := a.context(ctx)
	if err != nil {
		return nil, err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.provider != nil {
		return a.provider, nil
	}
	provider, err := oidc.NewProvider(oidcContext, common.OIDCIssuer)
	if err != nil {
		return nil, fmt.Errorf("discover OIDC provider: %w", err)
	}
	a.provider = provider
	return provider, nil
}

func oidcConfigured() bool {
	return common.OIDCIssuer != "" && common.OIDCClientID != ""
}

func oidcRedirectURL() string {
	return strings.TrimRight(common.Host, "/") + common.Base + "/api/auth/callback"
}

func oidcOAuthConfig(provider *oidc.Provider, redirectURL string) oauth2.Config {
	endpoint := provider.Endpoint()
	if common.OIDCClientSecret == "" {
		// Public PKCE clients authenticate with client_id in the token request body.
		endpoint.AuthStyle = oauth2.AuthStyleInParams
	}
	return oauth2.Config{
		ClientID:     common.OIDCClientID,
		ClientSecret: common.OIDCClientSecret,
		Endpoint:     endpoint,
		RedirectURL:  redirectURL,
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
	config := oidcOAuthConfig(provider, oidcRedirectURL())
	options := []oauth2.AuthCodeOption{
		oauth2.AccessTypeOffline,
		oidc.Nonce(nonce),
		oauth2.S256ChallengeOption(verifier),
	}
	return config.AuthCodeURL(state, options...), nil
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
	oidcContext, err := a.context(c.Request.Context())
	if err != nil {
		return nil, nil, err
	}
	provider, err := a.discoveredProvider(oidcContext)
	if err != nil {
		return nil, nil, err
	}
	config := oidcOAuthConfig(provider, oidcRedirectURL())
	options := []oauth2.AuthCodeOption{oauth2.VerifierOption(verifier)}
	token, err := config.Exchange(oidcContext, code, options...)
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
		return nonEmptyOIDCValues(values), nil
	}
	value, err := oidcStringClaim(raw, name)
	if err != nil {
		return nil, fmt.Errorf("OIDC claim %q must be a string or string array: %w", name, err)
	}
	return nonEmptyOIDCValues([]string{value}), nil
}

func nonEmptyOIDCValues(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			result = append(result, value)
		}
	}
	return result
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
		Issuer:     common.OIDCIssuer,
		OIDCGroups: append(model.SliceString(nil), claims.Groups...),
		Sub:        claims.Subject,
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
	staleBefore := time.Now().Add(-time.Duration(common.CookieExpirationSeconds) * time.Second)
	if err := model.DeleteInactiveOIDCSessions(staleBefore); err != nil {
		return fmt.Errorf("delete inactive OIDC sessions: %w", err)
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
	for _, allowed := range common.PlatformAdminSubjects {
		if user.Sub == allowed {
			return true
		}
	}
	for _, actual := range user.OIDCGroups {
		for _, allowed := range common.PlatformAdminGroups {
			if actual == allowed {
				return true
			}
		}
	}
	return false
}

func (a *oidcAuthenticator) authenticatedSession(c *gin.Context) (*model.User, string, uint, error) {
	opaqueToken, err := c.Cookie(sessionCookieName)
	if err != nil || opaqueToken == "" {
		return nil, "", 0, errors.New("session cookie is missing")
	}
	session, err := model.GetOIDCSessionByHash(hashOpaqueValue(opaqueToken))
	if err != nil {
		return nil, "", 0, errors.New("session not found")
	}
	user, err := model.GetUserByIDCached(uint64(session.UserID))
	if err != nil {
		_ = model.DeleteOIDCSession(session)
		return nil, "", 0, errors.New("OIDC user not found")
	}
	if user.Issuer != common.OIDCIssuer {
		_ = model.DeleteOIDCSession(session)
		return nil, "", 0, errors.New("OIDC session issuer no longer matches the configured issuer")
	}
	idToken, accessToken, err := a.sessionTokensForLoadedSession(c.Request.Context(), session)
	if err != nil {
		return nil, "", 0, err
	}
	c.Set(accessTokenContextKey, accessToken)
	return user, idToken, session.ID, nil
}

func (a *oidcAuthenticator) idTokenForSession(ctx context.Context, sessionID uint) (string, error) {
	if sessionID == 0 {
		return "", &SessionCredentialError{Err: errors.New("OIDC session is missing"), Permanent: true}
	}
	session, err := model.GetOIDCSessionByID(model.DB.WithContext(ctx), sessionID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", &SessionCredentialError{Err: errors.New("OIDC session no longer exists"), Permanent: true}
		}
		return "", err
	}
	idToken, _, err := a.sessionTokensForLoadedSession(ctx, session)
	return idToken, err
}

func (a *oidcAuthenticator) sessionTokensForLoadedSession(ctx context.Context, session *model.OIDCSession) (string, string, error) {
	if session == nil || session.ID == 0 {
		return "", "", &SessionCredentialError{Err: errors.New("OIDC session is missing"), Permanent: true}
	}
	if time.Until(session.ExpiresAt) > time.Minute {
		return string(session.IDToken), string(session.AccessToken), nil
	}

	refreshLock := &a.sessionRefreshLocks[session.ID%sessionLockShards]
	refreshLock.Lock()
	defer refreshLock.Unlock()
	refreshCtx, cancelRefresh := oidcRefreshContext(ctx)
	defer cancelRefresh()

	var rawIDToken string
	var accessToken string
	err := model.DB.WithContext(refreshCtx).Transaction(func(tx *gorm.DB) error {
		current, err := model.GetOIDCSessionByID(tx.Clauses(clause.Locking{Strength: "UPDATE"}), session.ID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return &SessionCredentialError{Err: errors.New("OIDC session no longer exists"), Permanent: true}
			}
			return err
		}
		if time.Until(current.ExpiresAt) <= time.Minute {
			if err := a.refreshSession(refreshCtx, tx, current); err != nil {
				return err
			}
		}
		rawIDToken = string(current.IDToken)
		accessToken = string(current.AccessToken)
		return nil
	})
	var credentialError *SessionCredentialError
	if errors.As(err, &credentialError) && credentialError.Permanent {
		_ = model.RevokeOIDCSession(session.ID, "OIDC authorization expired; re-enable this task to authorize it again")
	}
	return rawIDToken, accessToken, err
}

func oidcRefreshContext(requestContext context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(requestContext), oidcRefreshTimeout)
}

func (a *oidcAuthenticator) refreshSession(ctx context.Context, db *gorm.DB, session *model.OIDCSession) error {
	if session.RefreshToken == "" {
		return &SessionCredentialError{Err: errors.New("OIDC session cannot be refreshed"), Permanent: true}
	}
	oidcContext, err := a.context(ctx)
	if err != nil {
		return err
	}
	provider, err := a.discoveredProvider(oidcContext)
	if err != nil {
		return err
	}
	config := oidcOAuthConfig(provider, oidcRedirectURL())
	current := &oauth2.Token{
		AccessToken:  string(session.AccessToken),
		RefreshToken: string(session.RefreshToken),
		Expiry:       time.Now().Add(-time.Minute),
	}
	refreshed, err := config.TokenSource(oidcContext, current).Token()
	if err != nil {
		var retrieveError *oauth2.RetrieveError
		permanent := errors.As(err, &retrieveError) && (retrieveError.ErrorCode == "invalid_grant" || retrieveError.ErrorCode == "invalid_client")
		return &SessionCredentialError{Err: fmt.Errorf("refresh OIDC token: %w", err), Permanent: permanent}
	}
	rawIDToken, ok := refreshed.Extra("id_token").(string)
	if !ok || rawIDToken == "" {
		return errors.New("OIDC refresh did not return an ID token")
	}
	claims, idToken, err := a.verifyIDToken(oidcContext, provider, rawIDToken, "")
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
	if err := model.FindWithSubOrUpsertUserDB(db, updatedUser); err != nil {
		return err
	}
	refreshToken := refreshed.RefreshToken
	if refreshToken == "" {
		refreshToken = string(session.RefreshToken)
	}
	expiresAt := sessionExpiry(refreshed, idToken)
	if err := model.UpdateOIDCSession(db, session, map[string]any{
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
