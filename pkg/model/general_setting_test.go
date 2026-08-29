package model

import (
	"testing"

	"github.com/realmroot/lightkite/pkg/common"
)

func TestDefaultGeneralNodeTerminalImageValue(t *testing.T) {
	original := common.NodeTerminalImage
	t.Cleanup(func() {
		common.NodeTerminalImage = original
	})

	common.NodeTerminalImage = "  custom/node-terminal:1.0  "
	if got := DefaultGeneralNodeTerminalImageValue(); got != "custom/node-terminal:1.0" {
		t.Fatalf("DefaultGeneralNodeTerminalImageValue() = %q, want %q", got, "custom/node-terminal:1.0")
	}

	common.NodeTerminalImage = "   "
	if got := DefaultGeneralNodeTerminalImageValue(); got != DefaultGeneralNodeTerminalImage {
		t.Fatalf("DefaultGeneralNodeTerminalImageValue() = %q, want %q", got, DefaultGeneralNodeTerminalImage)
	}
}

func TestApplyRuntimeGeneralSetting(t *testing.T) {
	originalAnalytics := common.EnableAnalytics
	originalVersionCheck := common.EnableVersionCheck
	originalVersionCheckDisabled := common.VersionCheckDisabledByEnv
	t.Cleanup(func() {
		common.EnableAnalytics = originalAnalytics
		common.EnableVersionCheck = originalVersionCheck
		common.VersionCheckDisabledByEnv = originalVersionCheckDisabled
	})
	common.VersionCheckDisabledByEnv = false

	applyRuntimeGeneralSetting(&GeneralSetting{
		EnableAnalytics:    true,
		EnableVersionCheck: false,
	})

	if !common.EnableAnalytics {
		t.Fatalf("EnableAnalytics = %v, want true", common.EnableAnalytics)
	}
	if common.EnableVersionCheck {
		t.Fatalf("EnableVersionCheck = %v, want false", common.EnableVersionCheck)
	}

	applyRuntimeGeneralSetting(nil)
	if !common.EnableAnalytics {
		t.Fatalf("nil setting changed EnableAnalytics")
	}
	if common.EnableVersionCheck {
		t.Fatalf("nil setting changed EnableVersionCheck")
	}
}

func TestApplyRuntimeGeneralSettingHonorsOperatorVersionCheckDisable(t *testing.T) {
	originalVersionCheck := common.EnableVersionCheck
	originalVersionCheckDisabled := common.VersionCheckDisabledByEnv
	t.Cleanup(func() {
		common.EnableVersionCheck = originalVersionCheck
		common.VersionCheckDisabledByEnv = originalVersionCheckDisabled
	})
	common.VersionCheckDisabledByEnv = true
	common.EnableVersionCheck = false

	applyRuntimeGeneralSetting(&GeneralSetting{EnableVersionCheck: true})

	if common.EnableVersionCheck {
		t.Fatal("database setting overrode DISABLE_VERSION_CHECK")
	}
}
