package internal

import (
	"errors"
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
		t.Fatalf("migrate test schema: %v", err)
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
    tlsServerName: api.internal
    connectionMode: direct
    prometheusURL: http://prometheus.monitoring.svc:9090
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
	sections, err := applyConfig("fixture.yaml", config)
	if err != nil {
		t.Fatalf("applyConfig() error = %v", err)
	}
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

func TestLoadConfigRejectsInvalidCatalogWithoutReplacingCurrentState(t *testing.T) {
	setupTestDB(t)
	current := &model.Cluster{
		Name: "current", APIServerURL: "https://current.example.test", ConnectionMode: "direct", Enable: true,
	}
	if err := model.AddCluster(current); err != nil {
		t.Fatalf("seed current cluster: %v", err)
	}
	path := writeConfigFixture(t, `clusters:
  - name: invalid
    apiServerUrl: http://insecure.example.test
`)

	if err := LoadConfigFromFile(path); err == nil {
		t.Fatal("LoadConfigFromFile() accepted invalid cluster metadata")
	}
	clusters, err := model.ListClusters()
	if err != nil {
		t.Fatalf("list clusters: %v", err)
	}
	if len(clusters) != 1 || clusters[0].ID != current.ID || clusters[0].Name != current.Name {
		t.Fatalf("invalid configuration mutated catalog: %#v", clusters)
	}
}

func TestApplyClustersReconcilesCatalogWithoutChangingStableIdentity(t *testing.T) {
	setupTestDB(t)
	retained := &model.Cluster{
		Name: "retained", APIServerURL: "https://old.example.test", ConnectionMode: "direct", Enable: true,
	}
	removed := &model.Cluster{
		Name: "removed", APIServerURL: "https://removed.example.test", ConnectionMode: "direct", Enable: true,
	}
	if err := model.AddCluster(retained); err != nil {
		t.Fatalf("seed retained cluster: %v", err)
	}
	if err := model.AddCluster(removed); err != nil {
		t.Fatalf("seed removed cluster: %v", err)
	}
	if err := applyClusters([]ClusterConfig{{
		Name: "retained", Description: "updated", APIServerURL: "https://new.example.test", Default: true,
	}}); err != nil {
		t.Fatalf("applyClusters() error = %v", err)
	}

	updated, err := model.GetClusterByName("retained")
	if err != nil {
		t.Fatalf("load retained cluster: %v", err)
	}
	if updated.ID != retained.ID {
		t.Fatalf("retained cluster ID = %d, want stable ID %d", updated.ID, retained.ID)
	}
	if updated.Description != "updated" || updated.APIServerURL != "https://new.example.test" || !updated.IsDefault {
		t.Fatalf("retained cluster was not updated: %#v", updated)
	}
	if _, err := model.GetClusterByName("removed"); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("removed cluster lookup error = %v, want record not found", err)
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
		{name: "tunnel requires enrollment API", cluster: ClusterConfig{Name: "prod", ConnectionMode: "tunnel"}},
		{name: "credential bearing API URL", cluster: ClusterConfig{Name: "prod", APIServerURL: "https://user@api.example.test"}},
		{name: "external Prometheus URL", cluster: ClusterConfig{Name: "prod", APIServerURL: "https://api.example.test", PrometheusURL: "https://prometheus.example.test"}},
		{name: "invalid CA", cluster: ClusterConfig{Name: "prod", APIServerURL: "https://api.example.test", CABundle: "invalid"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := applyClusters([]ClusterConfig{test.cluster}); err == nil {
				t.Fatal("expected invalid cluster metadata to fail")
			}
		})
	}
	if err := applyClusters([]ClusterConfig{
		{Name: "first", APIServerURL: "https://first.example.test", Default: true},
		{Name: "second", APIServerURL: "https://second.example.test", Default: true},
	}); err == nil {
		t.Fatal("multiple default clusters were accepted")
	}
	if err := applyClusters([]ClusterConfig{
		{Name: "duplicate", APIServerURL: "https://first.example.test"},
		{Name: "duplicate", APIServerURL: "https://second.example.test"},
	}); err == nil {
		t.Fatal("duplicate cluster names were accepted")
	}
}
