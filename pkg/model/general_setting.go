package model

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"strings"

	"github.com/zxh326/kite/pkg/common"
	"gorm.io/gorm"
	"k8s.io/klog/v2"
)

const DefaultGeneralKubectlImage = "alpine/kubectl:1.36.3"
const DefaultGeneralNodeTerminalImage = "busybox:1.37.0"
const DefaultGeneralClusterAgentImage = ""

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

func DefaultGeneralClusterAgentImageValue() string {
	image := strings.TrimSpace(common.ClusterAgentImage)
	if image == "" {
		return DefaultGeneralClusterAgentImage
	}
	return image
}

type GeneralSetting struct {
	Model
	KubectlEnabled          bool         `json:"kubectlEnabled" gorm:"column:kubectl_enabled;type:boolean;not null;default:true"`
	KubectlImage            string       `json:"kubectlImage" gorm:"column:kubectl_image;type:varchar(255);not null;default:'alpine/kubectl:1.36.3'"`
	NodeTerminalImage       string       `json:"nodeTerminalImage" gorm:"column:node_terminal_image;type:varchar(255);not null;default:'busybox:1.37.0'"`
	ClusterAgentImage       string       `json:"clusterAgentImage" gorm:"column:cluster_agent_image;type:varchar(255);not null;default:''"`
	EnableAnalytics         bool         `json:"enableAnalytics" gorm:"column:enable_analytics;type:boolean;not null;default:false"`
	EnableVersionCheck      bool         `json:"enableVersionCheck" gorm:"column:enable_version_check;type:boolean;not null;default:true"`
	LoginPrompt             string       `json:"loginPrompt" gorm:"column:login_prompt;type:text"`
	JWTSecret               SecretString `json:"-" gorm:"column:jwt_secret;type:text"`
	GlobalSidebarPreference string       `json:"-" gorm:"column:global_sidebar_preference;type:text"`
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
		if setting.ClusterAgentImage == "" {
			setting.ClusterAgentImage = DefaultGeneralClusterAgentImageValue()
			updates["cluster_agent_image"] = setting.ClusterAgentImage
		}
		if err := ensureJWTSecret(&setting, updates); err != nil {
			return nil, err
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
		ClusterAgentImage:  DefaultGeneralClusterAgentImageValue(),
		EnableAnalytics:    common.EnableAnalytics,
		EnableVersionCheck: common.EnableVersionCheck,
	}
	if err := ensureJWTSecret(&setting, nil); err != nil {
		return nil, err
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

func ensureJWTSecret(setting *GeneralSetting, updates map[string]interface{}) error {
	storedSecret := strings.TrimSpace(string(setting.JWTSecret))
	configuredSecret := strings.TrimSpace(common.JwtSecret)

	switch {
	case configuredSecret != "" && configuredSecret != common.DefaultJWTSecret:
		if storedSecret != configuredSecret {
			setting.JWTSecret = SecretString(configuredSecret)
			if updates != nil {
				updates["jwt_secret"] = setting.JWTSecret
			}
		}
		common.JwtSecret = configuredSecret
		return nil
	case storedSecret != "" && storedSecret != common.DefaultJWTSecret:
		common.JwtSecret = storedSecret
		return nil
	default:
		generatedSecret, err := generateJWTSecret()
		if err != nil {
			return err
		}
		setting.JWTSecret = SecretString(generatedSecret)
		common.JwtSecret = generatedSecret
		if updates != nil {
			updates["jwt_secret"] = setting.JWTSecret
		}
		klog.Warningf("JWT secret is using the insecure default value, generated a random secret and stored it in general setting")
		return nil
	}
}

func generateJWTSecret() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
