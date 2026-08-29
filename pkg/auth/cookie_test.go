package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/zxh326/kite/pkg/common"
	"github.com/zxh326/kite/pkg/model"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestTransientSessionError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "temporary credential failure", err: &SessionCredentialError{Err: errors.New("provider unavailable")}, want: true},
		{name: "permanent credential failure", err: &SessionCredentialError{Err: errors.New("invalid grant"), Permanent: true}, want: false},
		{name: "request canceled", err: context.Canceled, want: true},
		{name: "deadline exceeded", err: context.DeadlineExceeded, want: true},
		{name: "unrelated failure", err: errors.New("database corrupt"), want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := transientSessionError(test.err); got != test.want {
				t.Fatalf("transientSessionError() = %t, want %t", got, test.want)
			}
		})
	}
}

func TestSetCookieSecureUsesCanonicalHostAndExactLifetime(t *testing.T) {
	originalHost := common.Host
	t.Cleanup(func() { common.Host = originalHost })
	gin.SetMode(gin.TestMode)

	t.Run("HTTPS host sets exact secure lifetime", func(t *testing.T) {
		common.Host = "https://kite.example.test"
		cookie := emittedCookie(t, 600, "")
		if !cookie.Secure || !cookie.HttpOnly || cookie.MaxAge != 600 || cookie.SameSite != http.SameSiteLaxMode {
			t.Fatalf("cookie = %#v", cookie)
		}
	})

	t.Run("deletion remains immediate", func(t *testing.T) {
		common.Host = "https://kite.example.test"
		cookie := emittedCookie(t, -1, "")
		if cookie.MaxAge >= 0 {
			t.Fatalf("deletion MaxAge = %d, want negative", cookie.MaxAge)
		}
	})

	t.Run("forwarded proto cannot upgrade loopback cookie", func(t *testing.T) {
		common.Host = "http://127.0.0.1:8080"
		cookie := emittedCookie(t, 600, "https")
		if cookie.Secure {
			t.Fatal("untrusted forwarded proto changed the canonical cookie policy")
		}
	})
}

func emittedCookie(t *testing.T, maxAge int, forwardedProto string) *http.Cookie {
	t.Helper()
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	if forwardedProto != "" {
		context.Request.Header.Set("X-Forwarded-Proto", forwardedProto)
	}
	setCookieSecure(context, "test_cookie", "value", maxAge)
	result := recorder.Result()
	t.Cleanup(func() { _ = result.Body.Close() })
	cookies := result.Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookies = %#v", cookies)
	}
	return cookies[0]
}

func TestRefreshTokenRenewsBrowserCookieAndServerSessionActivity(t *testing.T) {
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
	previousHost := common.Host
	model.DB = db
	common.KiteEncryptKey = "session-renewal-test-key"
	common.OIDCIssuer = "https://identity.example.test"
	common.Host = "https://kite.example.test"
	t.Cleanup(func() {
		model.DB = previousDB
		common.KiteEncryptKey = previousKey
		common.OIDCIssuer = previousIssuer
		common.Host = previousHost
	})

	user := model.User{Issuer: common.OIDCIssuer, Sub: "subject-1", Username: "alice"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	model.InvalidateUserCache(uint64(user.ID))
	t.Cleanup(func() { model.InvalidateUserCache(uint64(user.ID)) })

	opaqueToken := "opaque-browser-session"
	session := model.OIDCSession{
		TokenHash:   hashOpaqueValue(opaqueToken),
		UserID:      user.ID,
		IDToken:     model.SecretString("current-id-token"),
		AccessToken: model.SecretString("current-access-token"),
		ExpiresAt:   time.Now().Add(time.Hour),
	}
	if err := db.Create(&session).Error; err != nil {
		t.Fatal(err)
	}
	lastActivity := time.Now().Add(-time.Hour).UTC()
	if err := db.Model(&session).UpdateColumn("updated_at", lastActivity).Error; err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "/api/auth/refresh", nil)
	context.Request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: opaqueToken})
	(&AuthHandler{oidc: &oidcAuthenticator{}}).RefreshToken(context)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	response := recorder.Result()
	t.Cleanup(func() { _ = response.Body.Close() })
	cookies := response.Cookies()
	if len(cookies) != 1 || cookies[0].Name != sessionCookieName || cookies[0].Value != opaqueToken {
		t.Fatalf("renewed cookies = %#v", cookies)
	}
	if cookies[0].MaxAge != common.CookieExpirationSeconds || !cookies[0].HttpOnly || !cookies[0].Secure {
		t.Fatalf("renewed cookie = %#v", cookies[0])
	}

	var reloaded model.OIDCSession
	if err := db.First(&reloaded, session.ID).Error; err != nil {
		t.Fatal(err)
	}
	if !reloaded.UpdatedAt.After(lastActivity) {
		t.Fatalf("session activity was not renewed: before=%s after=%s", lastActivity, reloaded.UpdatedAt)
	}
}
