package cluster

import (
	"encoding/base64"
	"testing"

	"github.com/zxh326/kite/pkg/model"
)

func TestBaseRESTConfigContainsOnlyClusterConnectionMetadata(t *testing.T) {
	manager := &ClusterManager{}
	ca := "-----BEGIN CERTIFICATE-----\ntest\n-----END CERTIFICATE-----"
	config, generation, err := manager.baseRESTConfig(&model.Cluster{
		Name:           "production",
		APIServerURL:   "https://api.example.com:6443",
		CABundle:       base64.StdEncoding.EncodeToString([]byte(ca)),
		TLSServerName:  "api.internal",
		ConnectionMode: "direct",
		Config:         model.SecretString("legacy-secret-that-must-not-be-used"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if config.Host != "https://api.example.com:6443" || config.BearerToken != "" || generation != 0 {
		t.Fatalf("unexpected config: %#v", config)
	}
	if string(config.CAData) != ca || config.ServerName != "api.internal" {
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
