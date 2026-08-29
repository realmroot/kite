package internal

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"strings"

	"github.com/zxh326/kite/pkg/cluster"
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

func LoadConfigFromFile(path string) error {
	if path == "" {
		return nil
	}

	cfg, _, err := readConfigFile(path)
	if err != nil {
		return err
	}

	sections, err := applyConfig(path, cfg)
	if err != nil {
		return fmt.Errorf("apply configuration file %s: %w", path, err)
	}
	common.SetManagedSections(sections)
	return nil
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

func applyConfig(path string, cfg *KiteConfig) (AppliedSections, error) {
	sections := AppliedSections{}
	klog.Infof("Loading configuration from file: %s", path)
	if cfg.Clusters == nil {
		return sections, nil
	}
	if err := applyClusters(cfg.Clusters); err != nil {
		return nil, fmt.Errorf("apply cluster catalog: %w", err)
	}
	sections["clusters"] = true
	klog.Infof("Applied %d cluster(s) from config file", len(cfg.Clusters))
	return sections, nil
}

func applyClusters(clusters []ClusterConfig) error {
	defaultCount := 0
	configuredNames := make(map[string]struct{}, len(clusters))
	for i := range clusters {
		value := &clusters[i]
		value.Name = strings.TrimSpace(value.Name)
		value.APIServerURL = strings.TrimSpace(value.APIServerURL)
		value.CABundle = strings.TrimSpace(value.CABundle)
		value.TLSServerName = strings.TrimSpace(value.TLSServerName)
		value.PrometheusURL = strings.TrimSpace(value.PrometheusURL)
		if value.ConnectionMode == "" {
			value.ConnectionMode = "direct"
		}
		if value.ConnectionMode != "direct" {
			return fmt.Errorf("cluster %q: declarative catalog supports direct mode only; create tunnel clusters through the admin API", value.Name)
		}
		if value.Name == "" {
			return fmt.Errorf("cluster name is required")
		}
		if _, exists := configuredNames[value.Name]; exists {
			return fmt.Errorf("cluster %q is configured more than once", value.Name)
		}
		configuredNames[value.Name] = struct{}{}
		if err := cluster.ValidateDirectClusterMetadata(value.APIServerURL, value.CABundle, value.TLSServerName, value.PrometheusURL); err != nil {
			return fmt.Errorf("cluster %q: %w", value.Name, err)
		}
		if value.Default {
			defaultCount++
		}
	}
	if defaultCount > 1 {
		return fmt.Errorf("cluster catalog can contain at most one default cluster")
	}

	return model.DB.Transaction(func(tx *gorm.DB) error {
		var existingClusters []model.Cluster
		if err := tx.Find(&existingClusters).Error; err != nil {
			return fmt.Errorf("load existing cluster catalog: %w", err)
		}
		existingByName := make(map[string]*model.Cluster, len(existingClusters))
		for i := range existingClusters {
			existingByName[existingClusters[i].Name] = &existingClusters[i]
		}

		for _, value := range clusters {
			existing := existingByName[value.Name]
			if existing == nil {
				entry := &model.Cluster{
					Name:           value.Name,
					Description:    value.Description,
					APIServerURL:   value.APIServerURL,
					CABundle:       value.CABundle,
					TLSServerName:  value.TLSServerName,
					ConnectionMode: value.ConnectionMode,
					PrometheusURL:  value.PrometheusURL,
					IsDefault:      value.Default,
					Enable:         true,
				}
				if err := tx.Create(entry).Error; err != nil {
					return fmt.Errorf("create configured cluster %q: %w", value.Name, err)
				}
				continue
			}

			updates := map[string]any{
				"description":               value.Description,
				"api_server_url":            value.APIServerURL,
				"ca_bundle":                 value.CABundle,
				"tls_server_name":           value.TLSServerName,
				"connection_mode":           value.ConnectionMode,
				"prometheus_url":            value.PrometheusURL,
				"in_cluster":                false,
				"cluster_agent":             false,
				"cluster_agent_token_hash":  "",
				"cluster_agent_public_key":  "",
				"cluster_agent_private_key": "",
				"is_default":                value.Default,
				"enable":                    true,
			}
			if err := tx.Model(existing).Updates(updates).Error; err != nil {
				return fmt.Errorf("update configured cluster %q: %w", value.Name, err)
			}
			delete(existingByName, value.Name)
		}

		for name, existing := range existingByName {
			if err := tx.Unscoped().Delete(existing).Error; err != nil {
				return fmt.Errorf("delete cluster removed from configuration %q: %w", name, err)
			}
		}
		return nil
	})
}
