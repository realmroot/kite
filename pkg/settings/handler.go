package settings

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/zxh326/kite/pkg/common"
	"github.com/zxh326/kite/pkg/model"
)

func HandleGetGeneralSetting(c *gin.Context) {
	setting, err := model.GetGeneralSetting()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to load general setting: %v", err)})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"kubectlEnabled":      setting.KubectlEnabled,
		"kubectlImage":        setting.KubectlImage,
		"nodeTerminalImage":   setting.NodeTerminalImage,
		"enableAnalytics":     setting.EnableAnalytics,
		"analyticsConfigured": common.AnalyticsConfigured(),
		"enableVersionCheck":  setting.EnableVersionCheck,
		"loginPrompt":         setting.LoginPrompt,
	})
}

type UpdateGeneralSettingRequest struct {
	KubectlEnabled     *bool   `json:"kubectlEnabled"`
	KubectlImage       *string `json:"kubectlImage"`
	NodeTerminalImage  *string `json:"nodeTerminalImage"`
	EnableAnalytics    *bool   `json:"enableAnalytics"`
	EnableVersionCheck *bool   `json:"enableVersionCheck"`
	LoginPrompt        *string `json:"loginPrompt"`
}

func HandleUpdateGeneralSetting(c *gin.Context) {
	var req UpdateGeneralSettingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Invalid request: %v", err)})
		return
	}
	currentSetting, err := model.GetGeneralSetting()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to load general setting: %v", err)})
		return
	}

	kubectlEnabled := currentSetting.KubectlEnabled
	if req.KubectlEnabled != nil {
		kubectlEnabled = *req.KubectlEnabled
	}
	kubectlImage := strings.TrimSpace(currentSetting.KubectlImage)
	if req.KubectlImage != nil {
		kubectlImage = strings.TrimSpace(*req.KubectlImage)
	}
	if kubectlEnabled && req.KubectlImage != nil && strings.TrimSpace(*req.KubectlImage) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "kubectlImage is required when kubectlEnabled is true"})
		return
	}
	if kubectlImage == "" {
		kubectlImage = model.DefaultGeneralKubectlImageValue()
	}
	nodeTerminalImage := strings.TrimSpace(currentSetting.NodeTerminalImage)
	if req.NodeTerminalImage != nil {
		nodeTerminalImage = strings.TrimSpace(*req.NodeTerminalImage)
	}
	if nodeTerminalImage == "" {
		nodeTerminalImage = model.DefaultGeneralNodeTerminalImageValue()
	}

	updates := map[string]interface{}{}
	if req.KubectlEnabled != nil {
		updates["kubectl_enabled"] = kubectlEnabled
	}
	if req.KubectlImage != nil {
		updates["kubectl_image"] = kubectlImage
	}
	if req.NodeTerminalImage != nil {
		updates["node_terminal_image"] = nodeTerminalImage
	}
	if req.EnableAnalytics != nil {
		if *req.EnableAnalytics && !common.AnalyticsConfigured() {
			c.JSON(http.StatusBadRequest, gin.H{"error": "analytics requires ANALYTICS_SCRIPT_URL and ANALYTICS_WEBSITE_ID to be configured by the operator"})
			return
		}
		updates["enable_analytics"] = *req.EnableAnalytics
	}
	if req.EnableVersionCheck != nil {
		updates["enable_version_check"] = *req.EnableVersionCheck
	}
	if req.LoginPrompt != nil {
		updates["login_prompt"] = strings.TrimSpace(*req.LoginPrompt)
	}
	updated, err := model.UpdateGeneralSetting(updates)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to update general setting: %v", err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"kubectlEnabled":      updated.KubectlEnabled,
		"kubectlImage":        updated.KubectlImage,
		"nodeTerminalImage":   updated.NodeTerminalImage,
		"enableAnalytics":     updated.EnableAnalytics,
		"analyticsConfigured": common.AnalyticsConfigured(),
		"enableVersionCheck":  updated.EnableVersionCheck,
		"loginPrompt":         updated.LoginPrompt,
	})
}
