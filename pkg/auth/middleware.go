package auth

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/zxh326/kite/pkg/common"
	"github.com/zxh326/kite/pkg/model"
)

func (h *AuthHandler) RequireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		user, idToken, err := h.oidc.authenticatedSession(c)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "Invalid or expired OIDC session",
			})
			setCookieSecure(c, sessionCookieName, "", -1)
			c.Abort()
			return
		}
		if platformAdmin(*user) {
			user.Roles = []common.Role{platformAdminRole()}
		}
		c.Set("user", *user)
		c.Set(idTokenContextKey, idToken)
		c.Next()
	}
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
