package model

import (
	"errors"
	"strings"

	"github.com/realmroot/lightkite/pkg/common"
	"gorm.io/gorm"
)

const DefaultGeneralKubectlImage = "alpine/kubectl:1.36.3"
const DefaultGeneralNodeTerminalImage = "busybox:1.37.0"

func DefaultGeneralNodeTerminalImageValue() string {
	image := strings.TrimSpace(common.NodeTerminalImage)
	if image == "" {
		return DefaultGeneralNodeTerminalImage
	}
	return image
}

func DefaultGeneralKubectlImageValue() string {
	image := strings.TrimSpace(common.KubectlTerminalImage)
	if image == "" {
		return DefaultGeneralKubectlImage
	}
	return image
}

type GeneralSetting struct {
	Model
	KubectlEnabled          bool   `json:"kubectlEnabled" gorm:"column:kubectl_enabled;type:boolean;not null;default:true"`
	KubectlImage            string `json:"kubectlImage" gorm:"column:kubectl_image;type:varchar(255);not null;default:'alpine/kubectl:1.36.3'"`
	NodeTerminalImage       string `json:"nodeTerminalImage" gorm:"column:node_terminal_image;type:varchar(255);not null;default:'busybox:1.37.0'"`
	EnableAnalytics         bool   `json:"enableAnalytics" gorm:"column:enable_analytics;type:boolean;not null;default:false"`
	EnableVersionCheck      bool   `json:"enableVersionCheck" gorm:"column:enable_version_check;type:boolean;not null;default:true"`
	LoginPrompt             string `json:"loginPrompt" gorm:"column:login_prompt;type:text"`
	GlobalSidebarPreference string `json:"-" gorm:"column:global_sidebar_preference;type:text"`
}

func GetGeneralSetting() (*GeneralSetting, error) {
	var setting GeneralSetting
	err := DB.First(&setting, 1).Error
	if err == nil {
		updates := map[string]interface{}{}
		if setting.KubectlImage == "" {
			setting.KubectlImage = DefaultGeneralKubectlImageValue()
			updates["kubectl_image"] = setting.KubectlImage
		}
		if setting.NodeTerminalImage == "" {
			setting.NodeTerminalImage = DefaultGeneralNodeTerminalImageValue()
			updates["node_terminal_image"] = setting.NodeTerminalImage
		}
		if len(updates) > 0 {
			if err := DB.Model(&setting).Updates(updates).Error; err != nil {
				return nil, err
			}
		}
		applyRuntimeGeneralSetting(&setting)
		return &setting, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	setting = GeneralSetting{
		Model:              Model{ID: 1},
		KubectlEnabled:     true,
		KubectlImage:       DefaultGeneralKubectlImageValue(),
		NodeTerminalImage:  DefaultGeneralNodeTerminalImageValue(),
		EnableAnalytics:    common.EnableAnalytics,
		EnableVersionCheck: common.EnableVersionCheck,
	}
	if err := DB.Create(&setting).Error; err != nil {
		return nil, err
	}
	applyRuntimeGeneralSetting(&setting)
	return &setting, nil
}

func UpdateGeneralSetting(updates map[string]interface{}) (*GeneralSetting, error) {
	setting, err := GetGeneralSetting()
	if err != nil {
		return nil, err
	}
	if err := DB.Model(setting).Updates(updates).Error; err != nil {
		return nil, err
	}
	if err := DB.First(setting, setting.ID).Error; err != nil {
		return nil, err
	}
	applyRuntimeGeneralSetting(setting)
	return setting, nil
}

func applyRuntimeGeneralSetting(setting *GeneralSetting) {
	if setting == nil {
		return
	}
	common.EnableAnalytics = setting.EnableAnalytics
	common.EnableVersionCheck = setting.EnableVersionCheck && !common.VersionCheckDisabledByEnv
}
