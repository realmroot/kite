package cluster

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"k8s.io/client-go/rest"

	"github.com/zxh326/kite/pkg/clusteragent"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func init() {
	if err := os.Setenv("MOCKEY_CHECK_GCFLAGS", "false"); err != nil {
		panic(err)
	}
}

func TestValidateKubernetesAPIServerURL(t *testing.T) {
	tests := []struct {
		value string
		valid bool
	}{
		{value: "https://api.example.test:6443", valid: true},
		{value: "https://gateway.example.test/kubernetes", valid: true},
		{value: "http://api.example.test", valid: false},
		{value: "https://admin:secret@api.example.test", valid: false},
		{value: "https://api.example.test?token=secret", valid: false},
		{value: "https://api.example.test#credential", valid: false},
	}
	for _, test := range tests {
		err := validateKubernetesAPIServerURL(test.value)
		if (err == nil) != test.valid {
			t.Errorf("validateKubernetesAPIServerURL(%q) error = %v, valid=%t", test.value, err, test.valid)
		}
	}
}

func TestInvalidateCatalogRuntimesClosesAndDropsCachedTransports(t *testing.T) {
	transport := &closeTrackingTransport{}
	manager := &ClusterManager{
		clusterAgentManager: clusteragent.NewManager(func() {}),
		runtimes: map[uint]*clusterRuntime{
			7: {transport: transport},
		},
	}

	manager.InvalidateCatalogRuntimes()

	if !transport.closed.Load() {
		t.Fatal("cached transport was not closed")
	}
	if len(manager.runtimes) != 0 {
		t.Fatalf("runtime cache contains %d item(s), want 0", len(manager.runtimes))
	}
}

func TestIsClusterLocalURL(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want bool
	}{
		{
			name: "cluster local dns name",
			url:  "http://prometheus.monitoring.svc.cluster.local:9090",
			want: true,
		},
		{
			name: "svc host with port",
			url:  "http://prometheus.monitoring.svc:9090",
			want: true,
		},
		{
			name: "external url",
			url:  "https://prometheus.example.com",
			want: false,
		},
		{
			name: "lookalike suffix",
			url:  "https://prometheus.monitoring.svc.attacker.example",
			want: false,
		},
		{
			name: "credentials are forbidden",
			url:  "https://user:password@prometheus.monitoring.svc:9090",
			want: false,
		},
		{
			name: "service path is forbidden",
			url:  "http://prometheus.monitoring.svc:9090/prometheus",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isClusterLocalURL(tt.url); got != tt.want {
				t.Fatalf("isClusterLocalURL(%q) = %v, want %v", tt.url, got, tt.want)
			}
		})
	}
}

func TestCreateK8sProxyTransportRejectsNonServiceTargets(t *testing.T) {
	config := &rest.Config{Host: "https://apiserver.example.com"}
	for _, target := range []string{
		"https://prometheus.example.com",
		"file://prometheus.monitoring.svc",
		"http://prometheus.monitoring.svc.attacker.example",
	} {
		if _, err := createK8sProxyTransport(config, http.DefaultTransport, target); err == nil {
			t.Fatalf("createK8sProxyTransport(%q) succeeded, want error", target)
		}
	}
}

func TestCreateK8sProxyTransport(t *testing.T) {
	k8sConfig := &rest.Config{
		Host: "https://apiserver.example.com",
		TLSClientConfig: rest.TLSClientConfig{
			Insecure: true,
		},
	}

	t.Run("uses explicit port", func(t *testing.T) {
		transport, err := createK8sProxyTransport(k8sConfig, http.DefaultTransport, "https://prometheus.monitoring.svc.cluster.local:9090")
		if err != nil {
			t.Fatalf("createK8sProxyTransport() error = %v", err)
		}
		if transport.namespace != "monitoring" {
			t.Fatalf("namespace = %q, want %q", transport.namespace, "monitoring")
		}
		if transport.svcName != "prometheus" {
			t.Fatalf("svcName = %q, want %q", transport.svcName, "prometheus")
		}
		if transport.port != "9090" {
			t.Fatalf("port = %q, want %q", transport.port, "9090")
		}
	})

	t.Run("defaults https port", func(t *testing.T) {
		transport, err := createK8sProxyTransport(k8sConfig, http.DefaultTransport, "https://prometheus.monitoring.svc.cluster.local")
		if err != nil {
			t.Fatalf("createK8sProxyTransport() error = %v", err)
		}
		if transport.port != "443" {
			t.Fatalf("port = %q, want %q", transport.port, "443")
		}
	})
}

func TestNewUserClientSetOnlyEnablesKubernetesAuthorizedPrometheus(t *testing.T) {
	config := &rest.Config{Host: "https://apiserver.example.test"}
	httpClient := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		return nil, errors.New("unexpected request")
	})}

	external, err := newUserClientSet("prod", config, httpClient, "https://prometheus.example.test", "user-id-token")
	if err != nil {
		t.Fatalf("external Prometheus metadata should not break Kubernetes access: %v", err)
	}
	if external.PromClient != nil {
		t.Fatal("external Prometheus must not bypass Kubernetes authorization")
	}
	if external.K8sClient.Configuration.BearerToken != "user-id-token" || config.BearerToken != "" {
		t.Fatal("request-scoped Helm config must preserve the user token without mutating shared cluster metadata")
	}

	inCluster, err := newUserClientSet("prod", config, httpClient, "http://prometheus.monitoring.svc.cluster.local:9090", "")
	if err != nil {
		t.Fatalf("create Kubernetes-authorized Prometheus client: %v", err)
	}
	if inCluster.PromClient == nil {
		t.Fatal("cluster-local Prometheus should use the Kubernetes service proxy")
	}
}

func TestK8sProxyTransportRoundTrip(t *testing.T) {
	var gotMethod string
	var gotScheme string
	var gotHost string
	var gotPath string

	transport := &k8sProxyTransport{
		transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			gotMethod = req.Method
			gotScheme = req.URL.Scheme
			gotHost = req.URL.Host
			gotPath = req.URL.Path
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader("ok")),
				Header:     make(http.Header),
			}, nil
		}),
		apiServerURL: "https://apiserver.example.com",
		namespace:    "monitoring",
		svcName:      "prometheus",
		port:         "443",
	}

	req := httptest.NewRequest(http.MethodGet, "http://example.com/api/v1/query", nil)
	resp, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip() error = %v", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("StatusCode = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if gotMethod != http.MethodGet {
		t.Fatalf("method = %q, want %q", gotMethod, http.MethodGet)
	}
	if gotScheme != "https" {
		t.Fatalf("scheme = %q, want %q", gotScheme, "https")
	}
	if gotHost != "apiserver.example.com" {
		t.Fatalf("host = %q, want %q", gotHost, "apiserver.example.com")
	}
	if gotPath != "/api/v1/namespaces/monitoring/services/prometheus:443/proxy/api/v1/query" {
		t.Fatalf("path = %q, want %q", gotPath, "/api/v1/namespaces/monitoring/services/prometheus:443/proxy/api/v1/query")
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
