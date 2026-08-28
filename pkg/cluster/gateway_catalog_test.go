package cluster

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/zxh326/kite/pkg/model"
	"gorm.io/gorm"
)

func TestGatewayCatalogProjectsCredentialFreeClusters(t *testing.T) {
	var authorization string
	var apiVersion string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorization = r.Header.Get("Authorization")
		apiVersion = r.Header.Get("API-Version")
		cluster := gatewayCluster{
			ID: "development", DisplayName: "Development", Description: "Local kind",
			APIServerURL: "", AccessMode: "connector",
			ConnectorID: "development", ConnectorURL: "https://connector.example.test", Enabled: true, Default: true, ResourceVersion: 7,
		}
		switch r.URL.Path {
		case "/api/catalog/clusters":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []gatewayCluster{cluster}, "pagination": map[string]any{"pageSize": 200},
			})
		case "/api/catalog/clusters/development":
			_ = json.NewEncoder(w).Encode(cluster)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	db, err := gorm.Open(sqlite.Open("file:kite-gateway-catalog?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Cluster{}, &model.ScheduledTask{}); err != nil {
		t.Fatal(err)
	}
	previous := model.DB
	model.DB = db
	t.Cleanup(func() { model.DB = previous })

	catalog, err := newGatewayCatalog(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	catalog.client = server.Client()
	manager := &ClusterManager{gatewayCatalog: catalog, runtimes: map[uint]*clusterRuntime{}}
	clusters, details, err := manager.syncGatewayCatalogWithDetails(context.Background(), "user-id-token")
	if err != nil {
		t.Fatal(err)
	}
	if len(clusters) != 1 || clusters[0].CatalogID != "development" || clusters[0].CatalogResourceVersion != 7 {
		t.Fatalf("projected clusters = %#v", clusters)
	}
	if clusters[0].APIServerURL != server.URL+"/clusters/development/kubernetes" || clusters[0].CABundle != "" {
		t.Fatalf("projection contains wrong gateway connection metadata: %#v", clusters[0])
	}
	if details["development"].APIServerURL != "" || details["development"].AccessMode != "connector" {
		t.Fatalf("catalog details were not preserved for management UI: %#v", details)
	}
	if authorization != "Bearer user-id-token" || apiVersion != gatewayAPIVersion {
		t.Fatalf("gateway request headers authorization=%q apiVersion=%q", authorization, apiVersion)
	}

	clientSet, err := manager.GetClientSet("Development", "user-id-token")
	if err != nil {
		t.Fatal(err)
	}
	if clientSet.Name != "Development" || clientSet.ClusterID != clusters[0].ID {
		t.Fatalf("client set = %#v", clientSet)
	}
}

func TestGatewayClusterIDIsStableDNSLabel(t *testing.T) {
	if got := gatewayClusterID(" Production / Toronto "); got != "production-toronto" {
		t.Fatalf("gatewayClusterID = %q", got)
	}
}

func TestGatewayCatalogPutUsesResourceInputRepresentation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{"id", "resourceVersion", "createdAt", "updatedAt"} {
			if _, found := body[forbidden]; found {
				t.Fatalf("write representation contains response-only field %q: %#v", forbidden, body)
			}
		}
		_ = json.NewEncoder(w).Encode(gatewayCluster{ID: "development", DisplayName: "Development", ResourceVersion: 1})
	}))
	t.Cleanup(server.Close)
	catalog, err := newGatewayCatalog(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := catalog.Put(context.Background(), "user-id-token", gatewayCluster{
		ID: "development", DisplayName: "Development", APIServerURL: "https://kubernetes.example.test", AccessMode: "connector",
		ConnectorID: "development", ConnectorURL: "https://connector.example.test", Enabled: true,
	}, true); err != nil {
		t.Fatal(err)
	}
}

func TestValidateGatewayAccessModes(t *testing.T) {
	if err := validateGatewayAccess("direct", "development", "https://api.example.test", "", "", ""); err != nil {
		t.Fatal(err)
	}
	if err := validateGatewayAccess("connector", "development", "", "", "development", "https://connector.example.test"); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name, mode, connectorID, connectorURL string
	}{
		{name: "connector API server", mode: "connector", connectorID: "development", connectorURL: "https://connector.example.test"},
		{name: "connector mismatched ID", mode: "connector", connectorID: "another-cluster", connectorURL: "https://connector.example.test"},
		{name: "connector insecure URL", mode: "connector", connectorID: "development", connectorURL: "http://connector.example.test"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := validateGatewayAccess(test.mode, "development", "https://api.example.test", "", test.connectorID, test.connectorURL); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}
