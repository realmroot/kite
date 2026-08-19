package auth

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/zxh326/kite/pkg/common"
	"github.com/zxh326/kite/pkg/model"
	"github.com/zxh326/kite/pkg/rbac"
)

func (h *AuthHandler) RequireAPIKeyAuth(c *gin.Context, token string) {
	keyPart := strings.SplitN(token, "-", 2)
	if len(keyPart) < 2 {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "Invalid API key",
		})
		c.Abort()
		return
	}
	id := keyPart[0]
	key := keyPart[1]
	dbID, err := strconv.ParseUint(id, 10, 64)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "Invalid API key",
		})
		c.Abort()
		return
	}
	apikey, err := model.GetUserByIDCached(dbID)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "Invalid API key",
		})
		c.Abort()
		return
	}
	if !apikey.Enabled || apikey.Provider != common.APIKeyProvider || key == "" || string(apikey.APIKey) == "" || key != string(apikey.APIKey) {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "Invalid API key",
		})
		c.Abort()
		return
	}
	_ = model.LoginUser(apikey)
	apikey.Roles = rbac.GetUserRoles(*apikey)
	c.Set("user", *apikey)
}

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
