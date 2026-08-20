package internal

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/zxh326/kite/pkg/common"
	"github.com/zxh326/kite/pkg/model"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupTestDB(t *testing.T) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	if err := db.AutoMigrate(&model.Cluster{}); err != nil {
		t.Fatalf("migrate clusters: %v", err)
	}
	oldDB := model.DB
	model.DB = db
	t.Cleanup(func() { model.DB = oldDB })
}

func writeConfigFixture(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func TestReadConfigAcceptsOnlyCredentialFreeClusterMetadata(t *testing.T) {
	path := writeConfigFixture(t, `clusters:
  - name: production
    apiServerUrl: ${TEST_API_SERVER}
    caBundle: test-ca
    tlsServerName: api.internal
    connectionMode: direct
    prometheusURL: https://prometheus.example.test
    default: true
`)
	t.Setenv("TEST_API_SERVER", "https://api.example.test")

	config, _, err := readConfigFile(path)
	if err != nil {
		t.Fatalf("readConfigFile() error = %v", err)
	}
	if len(config.Clusters) != 1 || config.Clusters[0].APIServerURL != "https://api.example.test" {
		t.Fatalf("unexpected config: %#v", config)
	}
}

func TestReadConfigRejectsLegacyIdentityAndRBACSections(t *testing.T) {
	for _, section := range []string{"superUser", "oauth", "ldap", "rbac"} {
		t.Run(section, func(t *testing.T) {
			path := writeConfigFixture(t, section+": {}\n")
			_, _, err := readConfigFile(path)
			if err == nil || !strings.Contains(err.Error(), "field "+section+" not found") {
				t.Fatalf("error = %v, want unsupported field", err)
			}
		})
	}
}

func TestApplyConfigReplacesOnlyClusterCatalog(t *testing.T) {
	setupTestDB(t)
	oldManaged := common.ManagedSections
	common.ManagedSections = map[string]bool{}
	t.Cleanup(func() { common.ManagedSections = oldManaged })

	if err := model.AddCluster(&model.Cluster{Name: "old", APIServerURL: "https://old.example.test", Enable: true}); err != nil {
		t.Fatalf("seed cluster: %v", err)
	}
	config := &KiteConfig{Clusters: []ClusterConfig{{
		Name: "new", APIServerURL: "https://new.example.test", Default: true,
	}}}
	sections := applyConfig("fixture.yaml", config)
	if !sections["clusters"] || len(sections) != 1 {
		t.Fatalf("managed sections = %#v", sections)
	}
	clusters, err := model.ListClusters()
	if err != nil {
		t.Fatalf("list clusters: %v", err)
	}
	if len(clusters) != 1 || clusters[0].Name != "new" || clusters[0].ConnectionMode != "direct" {
		t.Fatalf("clusters = %#v", clusters)
	}
}

func TestApplyClustersValidatesConnectionMetadata(t *testing.T) {
	setupTestDB(t)
	for _, test := range []struct {
		name    string
		cluster ClusterConfig
	}{
		{name: "missing name", cluster: ClusterConfig{APIServerURL: "https://api.example.test"}},
		{name: "missing direct API server", cluster: ClusterConfig{Name: "prod"}},
		{name: "unknown mode", cluster: ClusterConfig{Name: "prod", ConnectionMode: "kubeconfig"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := applyClusters([]ClusterConfig{test.cluster}); err == nil {
				t.Fatal("expected invalid cluster metadata to fail")
			}
		})
	}
}
