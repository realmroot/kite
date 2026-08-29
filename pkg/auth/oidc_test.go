package auth

import (
	"context"
	"encoding/json"
	"encoding/pem"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/zxh326/kite/pkg/common"
	"github.com/zxh326/kite/pkg/model"
	"golang.org/x/oauth2"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestOIDCConfiguredSupportsPublicPKCEClient(t *testing.T) {
	previousIssuer, previousID, previousSecret := common.OIDCIssuer, common.OIDCClientID, common.OIDCClientSecret
	common.OIDCIssuer, common.OIDCClientID, common.OIDCClientSecret = "https://identity.example.com", "public-client", ""
	t.Cleanup(func() {
		common.OIDCIssuer, common.OIDCClientID, common.OIDCClientSecret = previousIssuer, previousID, previousSecret
	})
	if !oidcConfigured() {
		t.Fatal("public Authorization Code + PKCE client should be configured without a secret")
	}
	config := oidcOAuthConfig(&oidc.Provider{}, "http://localhost/callback")
	if config.Endpoint.AuthStyle != oauth2.AuthStyleInParams {
		t.Fatalf("public client auth style = %v", config.Endpoint.AuthStyle)
	}
}

func TestOIDCCustomCATrustsPrivateIssuer(t *testing.T) {
	server := httptest.NewTLSServer(nil)
	t.Cleanup(server.Close)
	certificate := server.Certificate()
	caFile := t.TempDir() + "/oidc-ca.pem"
	if err := os.WriteFile(caFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificate.Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	previous := common.OIDCCAFile
	common.OIDCCAFile = caFile
	t.Cleanup(func() { common.OIDCCAFile = previous })

	authenticator := &oidcAuthenticator{}
	if _, err := authenticator.context(context.Background()); err != nil {
		t.Fatal(err)
	}
	response, err := authenticator.client.Get(server.URL)
	if err != nil {
		t.Fatalf("custom OIDC CA was not trusted: %v", err)
	}
	if err := response.Body.Close(); err != nil {
		t.Fatalf("close OIDC CA test response: %v", err)
	}
}

func TestUserFromOIDCClaimsPreservesConfiguredIdentity(t *testing.T) {
	user := userFromOIDCClaims(&oidcClaims{
		Subject:  "user-123",
		Username: "alice",
		Name:     "Alice",
		Groups:   []string{"platform-admins", "developers"},
	})
	if user.Issuer != common.OIDCIssuer || user.Sub != "user-123" || user.Username != "alice" {
		t.Fatalf("user = %#v", user)
	}
	if len(user.OIDCGroups) != 2 || user.OIDCGroups[1] != "developers" {
		t.Fatalf("groups = %#v", user.OIDCGroups)
	}
}

func TestPlatformAdminComesOnlyFromConfiguredOIDCPrincipal(t *testing.T) {
	previous := common.PlatformAdminGroups
	previousSubjects := common.PlatformAdminSubjects
	common.PlatformAdminGroups = []string{"platform-admins"}
	common.PlatformAdminSubjects = []string{"person-123"}
	t.Cleanup(func() {
		common.PlatformAdminGroups = previous
		common.PlatformAdminSubjects = previousSubjects
	})

	if !platformAdmin(model.User{OIDCGroups: []string{"platform-admins"}}) {
		t.Fatal("configured OIDC group should grant catalog administration")
	}
	if platformAdmin(model.User{OIDCGroups: []string{"developers"}}) {
		t.Fatal("unconfigured group must not grant catalog administration")
	}
	if !platformAdmin(model.User{Sub: "person-123"}) {
		t.Fatal("configured OIDC subject should grant catalog administration")
	}
	if platformAdmin(model.User{Sub: "person-456"}) {
		t.Fatal("unconfigured OIDC subject must not grant catalog administration")
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

func TestOIDCClaimDecodingDropsEmptyGroups(t *testing.T) {
	groups, err := oidcStringListClaim(map[string]json.RawMessage{
		"groups": json.RawMessage(`["", " developers "]`),
	}, "groups")
	if err != nil || len(groups) != 1 || groups[0] != "developers" {
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

func TestIDTokenForSessionUsesCurrentUnexpiredCredential(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.OIDCSession{}); err != nil {
		t.Fatal(err)
	}
	previousDB := model.DB
	previousKey := common.KiteEncryptKey
	model.DB = db
	common.KiteEncryptKey = "test-encryption-key"
	t.Cleanup(func() {
		model.DB = previousDB
		common.KiteEncryptKey = previousKey
	})
	session := model.OIDCSession{
		TokenHash: "hash",
		UserID:    1,
		IDToken:   model.SecretString("current-id-token"),
		ExpiresAt: time.Now().Add(time.Hour),
	}
	if err := db.Create(&session).Error; err != nil {
		t.Fatal(err)
	}
	createdAt := session.UpdatedAt

	token, err := (&oidcAuthenticator{}).idTokenForSession(context.Background(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if token != "current-id-token" {
		t.Fatalf("token = %q", token)
	}
	var reloaded model.OIDCSession
	if err := db.First(&reloaded, session.ID).Error; err != nil {
		t.Fatal(err)
	}
	if !reloaded.UpdatedAt.Equal(createdAt) {
		t.Fatalf("unexpired token fast path wrote the session: before=%s after=%s", createdAt, reloaded.UpdatedAt)
	}
}

func TestSessionTokensUseCredentialsReloadedUnderRefreshLock(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.OIDCSession{}); err != nil {
		t.Fatal(err)
	}
	previousDB := model.DB
	previousKey := common.KiteEncryptKey
	model.DB = db
	common.KiteEncryptKey = "refresh-race-test-key"
	t.Cleanup(func() {
		model.DB = previousDB
		common.KiteEncryptKey = previousKey
	})

	current := model.OIDCSession{
		TokenHash:   "hash",
		UserID:      1,
		IDToken:     model.SecretString("current-id-token"),
		AccessToken: model.SecretString("current-access-token"),
		ExpiresAt:   time.Now().Add(time.Hour),
	}
	if err := db.Create(&current).Error; err != nil {
		t.Fatal(err)
	}
	stale := &model.OIDCSession{
		Model:       model.Model{ID: current.ID},
		IDToken:     model.SecretString("stale-id-token"),
		AccessToken: model.SecretString("stale-access-token"),
		ExpiresAt:   time.Now().Add(-time.Minute),
	}

	idToken, accessToken, err := (&oidcAuthenticator{}).sessionTokensForLoadedSession(context.Background(), stale)
	if err != nil {
		t.Fatal(err)
	}
	if idToken != "current-id-token" || accessToken != "current-access-token" {
		t.Fatalf("tokens = (%q, %q), want current database credentials", idToken, accessToken)
	}
}

func TestOIDCSessionRefreshLocksAreSharded(t *testing.T) {
	authenticator := &oidcAuthenticator{}
	first := &authenticator.sessionRefreshLocks[1%sessionLockShards]
	second := &authenticator.sessionRefreshLocks[2%sessionLockShards]
	sameSession := &authenticator.sessionRefreshLocks[1%sessionLockShards]
	if first == second {
		t.Fatal("different sessions unexpectedly share the same refresh lock shard")
	}
	if first != sameSession {
		t.Fatal("the same session did not resolve to a stable refresh lock")
	}
}

func TestOIDCRefreshContextOutlivesCanceledBrowserRequest(t *testing.T) {
	requestContext, cancelRequest := context.WithCancel(context.Background())
	cancelRequest()

	refreshContext, cancelRefresh := oidcRefreshContext(requestContext)
	defer cancelRefresh()
	if err := refreshContext.Err(); err != nil {
		t.Fatalf("refresh context inherited browser cancellation: %v", err)
	}
	deadline, ok := refreshContext.Deadline()
	if !ok {
		t.Fatal("refresh context has no deadline")
	}
	if remaining := time.Until(deadline); remaining <= 0 || remaining > oidcRefreshTimeout {
		t.Fatalf("refresh deadline remaining = %v, want within %v", remaining, oidcRefreshTimeout)
	}
}

func TestIDTokenForMissingSessionIsPermanent(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.OIDCSession{}); err != nil {
		t.Fatal(err)
	}
	previousDB := model.DB
	model.DB = db
	t.Cleanup(func() { model.DB = previousDB })

	_, err = (&oidcAuthenticator{}).idTokenForSession(context.Background(), 42)
	var credentialError *SessionCredentialError
	if !errors.As(err, &credentialError) || !credentialError.IsPermanent() {
		t.Fatalf("error = %#v, want permanent session credential error", err)
	}
}

func TestAuthenticatedSessionRejectsPreviousConfiguredIssuer(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.OIDCSession{}); err != nil {
		t.Fatal(err)
	}
	previousDB := model.DB
	previousKey := common.KiteEncryptKey
	previousIssuer := common.OIDCIssuer
	model.DB = db
	common.KiteEncryptKey = "issuer-change-test-key"
	common.OIDCIssuer = "https://current-issuer.example"
	t.Cleanup(func() {
		model.DB = previousDB
		common.KiteEncryptKey = previousKey
		common.OIDCIssuer = previousIssuer
	})

	user := model.User{Issuer: "https://previous-issuer.example", Sub: "subject", Username: "alice"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	model.InvalidateUserCache(uint64(user.ID))
	opaqueToken := "opaque-browser-session"
	session := model.OIDCSession{
		TokenHash: hashOpaqueValue(opaqueToken),
		UserID:    user.ID,
		IDToken:   model.SecretString("old-issuer-token"),
		ExpiresAt: time.Now().Add(time.Hour),
	}
	if err := db.Create(&session).Error; err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodGet, "/api/auth/user", nil)
	context.Request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: opaqueToken})
	if _, _, _, err := (&oidcAuthenticator{}).authenticatedSession(context); err == nil {
		t.Fatal("session from the previous configured issuer was accepted")
	}
	var sessionCount int64
	if err := db.Model(&model.OIDCSession{}).Count(&sessionCount).Error; err != nil {
		t.Fatal(err)
	}
	if sessionCount != 0 {
		t.Fatalf("old issuer session count = %d, want revoked", sessionCount)
	}
}
