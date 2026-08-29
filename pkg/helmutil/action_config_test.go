package helmutil

import (
	"bytes"
	"testing"

	"k8s.io/client-go/rest"
)

func TestRESTClientConfigPreservesUserOIDCCredentials(t *testing.T) {
	source := &rest.Config{
		Host:        "https://api.example.test:6443",
		BearerToken: "user-id-token",
		TLSClientConfig: rest.TLSClientConfig{
			CAData:     []byte("test-ca"),
			ServerName: "api.example.test",
		},
	}
	loader := (&restClientGetter{config: source, namespace: "team-a"}).ToRawKubeConfigLoader()
	actual, err := loader.ClientConfig()
	if err != nil {
		t.Fatal(err)
	}
	if actual.Host != source.Host || actual.BearerToken != source.BearerToken || actual.ServerName != source.ServerName || !bytes.Equal(actual.CAData, source.CAData) {
		t.Fatalf("client config lost user identity or TLS metadata: %#v", actual)
	}
	namespace, explicit, err := loader.Namespace()
	if err != nil || !explicit || namespace != "team-a" {
		t.Fatalf("namespace = %q, explicit = %t, err = %v", namespace, explicit, err)
	}
	raw, err := loader.RawConfig()
	if err != nil {
		t.Fatal(err)
	}
	if raw.AuthInfos["lightkite"].Token != source.BearerToken || !bytes.Equal(raw.Clusters["lightkite"].CertificateAuthorityData, source.CAData) {
		t.Fatalf("raw config lost user identity or CA: %#v", raw)
	}
}
