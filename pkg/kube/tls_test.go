package kube

import (
	"encoding/base64"
	"encoding/pem"
	"net/http/httptest"
	"testing"
)

func testCABundle(t *testing.T) []byte {
	t.Helper()
	server := httptest.NewTLSServer(nil)
	t.Cleanup(server.Close)
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})
}

func TestNormalizeCABundle(t *testing.T) {
	pemData := testCABundle(t)
	for _, value := range []string{string(pemData), base64.StdEncoding.EncodeToString(pemData)} {
		got, err := NormalizeCABundle(value)
		if err != nil {
			t.Fatal(err)
		}
		if err := ValidateCAData(got); err != nil {
			t.Fatal(err)
		}
	}
	for _, value := range []string{"not-base64", base64.StdEncoding.EncodeToString([]byte("not PEM")), "-----BEGIN PRIVATE KEY-----\nAAAA\n-----END PRIVATE KEY-----"} {
		if _, err := NormalizeCABundle(value); err == nil {
			t.Fatalf("invalid CA bundle %q was accepted", value)
		}
	}
}

func TestValidateTLSServerName(t *testing.T) {
	for _, value := range []string{"", "api.example.test", "127.0.0.1", "2001:db8::1"} {
		if err := ValidateTLSServerName(value); err != nil {
			t.Fatalf("ValidateTLSServerName(%q): %v", value, err)
		}
	}
	for _, value := range []string{"https://api.example.test", "bad_name", "*.example.test"} {
		if err := ValidateTLSServerName(value); err == nil {
			t.Fatalf("invalid TLS server name %q was accepted", value)
		}
	}
}
