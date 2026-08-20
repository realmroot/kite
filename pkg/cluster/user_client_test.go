package cluster

import (
	"encoding/base64"
	"encoding/pem"
	"net/http/httptest"
	"testing"

	"github.com/zxh326/kite/pkg/model"
)

func TestBaseRESTConfigContainsOnlyClusterConnectionMetadata(t *testing.T) {
	manager := &ClusterManager{}
	server := httptest.NewTLSServer(nil)
	t.Cleanup(server.Close)
	ca := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})
	config, generation, err := manager.baseRESTConfig(&model.Cluster{
		Name:           "production",
		APIServerURL:   "https://api.example.com:6443",
		CABundle:       base64.StdEncoding.EncodeToString(ca),
		TLSServerName:  "api.internal",
		ConnectionMode: "direct",
	})
	if err != nil {
		t.Fatal(err)
	}
	if config.Host != "https://api.example.com:6443" || config.BearerToken != "" || generation != 0 {
		t.Fatalf("unexpected config: %#v", config)
	}
	if string(config.CAData) != string(ca) || config.ServerName != "api.internal" {
		t.Fatalf("TLS metadata was not preserved: %#v", config.TLSClientConfig)
	}
	if config.Username != "" || config.Password != "" || len(config.CertData) != 0 || len(config.KeyData) != 0 || config.BearerTokenFile != "" {
		t.Fatalf("user config contains a non-OIDC credential: %#v", config)
	}
}

func TestUserRESTConfigRequiresOIDCIdentity(t *testing.T) {
	manager := &ClusterManager{}
	_, err := manager.GetClientSet("production", "")
	if err == nil || err.Error() != "OIDC ID token is required" {
		t.Fatalf("error = %v", err)
	}
}
