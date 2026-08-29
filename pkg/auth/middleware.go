package auth

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/zxh326/kite/pkg/model"
	"k8s.io/klog/v2"
)

const oidcSessionIDContextKey = "oidc-session-id"

func OIDCSessionID(c *gin.Context) (uint, bool) {
	value, exists := c.Get(oidcSessionIDContextKey)
	if !exists {
		return 0, false
	}
	sessionID, ok := value.(uint)
	return sessionID, ok && sessionID != 0
}

func (h *AuthHandler) RequireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		user, idToken, sessionID, err := h.oidc.authenticatedSession(c)
		if err != nil {
			klog.V(2).Infof("OIDC session authentication failed: %v", err)
			if transientSessionError(err) {
				c.JSON(http.StatusServiceUnavailable, gin.H{
					"error": "OIDC session refresh is temporarily unavailable",
				})
				c.Abort()
				return
			}
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "Invalid or expired OIDC session",
			})
			setCookieSecure(c, sessionCookieName, "", -1)
			c.Abort()
			return
		}
		c.Set("user", *user)
		c.Set("platform-admin", platformAdmin(*user))
		c.Set(idTokenContextKey, idToken)
		c.Set(oidcSessionIDContextKey, sessionID)
		c.Next()
	}
}

func transientSessionError(err error) bool {
	var credentialError *SessionCredentialError
	if errors.As(err, &credentialError) {
		return !credentialError.Permanent
	}
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

func (h *AuthHandler) RequireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		user, exists := c.Get("user")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "Not authenticated",
			})
			c.Abort()
			return
		}

		u := user.(model.User)
		if !platformAdmin(u) {
			c.JSON(http.StatusForbidden, gin.H{
				"error": "Admin role required",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}
