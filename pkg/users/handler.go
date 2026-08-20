package users

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/zxh326/kite/pkg/model"
	"k8s.io/klog/v2"
)

func UpdateSidebarPreference(c *gin.Context) {
	user := c.MustGet("user").(model.User)
	platformAdmin, _ := c.Get("platform-admin")
	if platformAdmin != true {
		setting, err := model.GetGeneralSetting()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load general setting"})
			return
		}
		if strings.TrimSpace(setting.GlobalSidebarPreference) != "" {
			c.JSON(http.StatusForbidden, gin.H{"error": "sidebar customization is disabled by global sidebar"})
			return
		}
	}
	var req struct {
		SidebarPreference string `json:"sidebar_preference" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	user.SidebarPreference = req.SidebarPreference
	if err := model.UpdateUser(&user); err != nil {
		klog.Errorf("failed to update sidebar preference for user %s: %v", user.Username, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update sidebar preference"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func UpdateGlobalSidebarPreference(c *gin.Context) {
	var req struct {
		SidebarPreference string `json:"sidebar_preference" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if _, err := model.UpdateGeneralSetting(map[string]interface{}{"global_sidebar_preference": req.SidebarPreference}); err != nil {
		klog.Errorf("failed to update global sidebar preference: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update global sidebar preference"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func ClearGlobalSidebarPreference(c *gin.Context) {
	if _, err := model.UpdateGeneralSetting(map[string]interface{}{"global_sidebar_preference": ""}); err != nil {
		klog.Errorf("failed to clear global sidebar preference: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to clear global sidebar preference"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}
