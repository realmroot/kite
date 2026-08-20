package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/zxh326/kite/pkg/common"
)

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
