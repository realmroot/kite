package internal

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"

	"github.com/zxh326/kite/pkg/common"
	"github.com/zxh326/kite/pkg/model"
	"gopkg.in/yaml.v3"
	"gorm.io/gorm"
	"k8s.io/klog/v2"
)

// KiteConfig is intentionally limited to transport-only cluster metadata.
// Identity provider and Kubernetes authorization policy live outside Kite.
type KiteConfig struct {
	Clusters []ClusterConfig `yaml:"clusters"`
}

type AppliedSections map[string]bool

type ClusterConfig struct {
	Name           string `yaml:"name"`
	Description    string `yaml:"description"`
	APIServerURL   string `yaml:"apiServerUrl"`
	CABundle       string `yaml:"caBundle"`
	TLSServerName  string `yaml:"tlsServerName"`
	ConnectionMode string `yaml:"connectionMode"`
	PrometheusURL  string `yaml:"prometheusURL"`
	Default        bool   `yaml:"default"`
}

func LoadConfigFromFile(path string) {
	if path == "" {
		return
	}

	cfg, _, err := readConfigFile(path)
	if err != nil {
		klog.Fatalf("%v", err)
		return
	}

	sections := applyConfig(path, cfg)
	common.SetManagedSections(sections)
}

func readConfigFile(path string) (*KiteConfig, [sha256.Size]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, [sha256.Size]byte{}, fmt.Errorf("failed to read config file %s: %w", path, err)
	}
	hash := sha256.Sum256(data)

	decoder := yaml.NewDecoder(bytes.NewBufferString(os.ExpandEnv(string(data))))
	decoder.KnownFields(true)
	var cfg KiteConfig
	if err := decoder.Decode(&cfg); err != nil {
		return nil, hash, fmt.Errorf("failed to parse config file %s: %w", path, err)
	}
	return &cfg, hash, nil
}

func applyConfig(path string, cfg *KiteConfig) AppliedSections {
	sections := AppliedSections{}
	klog.Infof("Loading configuration from file: %s", path)
	if cfg.Clusters == nil {
		return sections
	}
	if err := applyClusters(cfg.Clusters); err != nil {
		klog.Errorf("Failed to apply cluster config: %v", err)
		return sections
	}
	sections["clusters"] = true
	klog.Infof("Applied %d cluster(s) from config file", len(cfg.Clusters))
	return sections
}

func applyClusters(clusters []ClusterConfig) error {
	return model.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("1 = 1").Delete(&model.Cluster{}).Error; err != nil {
			return err
		}

		for _, value := range clusters {
			connectionMode := value.ConnectionMode
			if connectionMode == "" {
				connectionMode = "direct"
			}
			if connectionMode != "direct" && connectionMode != "tunnel" {
				return fmt.Errorf("cluster %q has unsupported connection mode %q", value.Name, connectionMode)
			}
			if value.Name == "" {
				return fmt.Errorf("cluster name is required")
			}
			if connectionMode == "direct" && value.APIServerURL == "" {
				return fmt.Errorf("cluster %q API server URL is required in direct mode", value.Name)
			}
			cluster := &model.Cluster{
				Name:           value.Name,
				Description:    value.Description,
				APIServerURL:   value.APIServerURL,
				CABundle:       value.CABundle,
				TLSServerName:  value.TLSServerName,
				ConnectionMode: connectionMode,
				ClusterAgent:   connectionMode == "tunnel",
				PrometheusURL:  value.PrometheusURL,
				IsDefault:      value.Default,
				Enable:         true,
			}
			if err := tx.Create(cluster).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
