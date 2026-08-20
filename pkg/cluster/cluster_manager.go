package cluster

import (
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/zxh326/kite/pkg/clusteragent"
	"github.com/zxh326/kite/pkg/kube"
	"github.com/zxh326/kite/pkg/model"
	"github.com/zxh326/kite/pkg/prometheus"
	"k8s.io/client-go/rest"
	kubetransport "k8s.io/client-go/transport"
)

type ClientSet struct {
	Name       string
	Version    string // Kubernetes version
	K8sClient  *kube.K8sClient
	PromClient *prometheus.Client

	DiscoveredPrometheusURL string
	prometheusURL           string
}

type ClusterManager struct {
	clusterAgentManager *clusteragent.Manager
	runtimeMu           sync.Mutex
	runtimes            map[uint]*clusterRuntime
	transportFor        func(*rest.Config) (http.RoundTripper, error)
}

type clusterRuntime struct {
	signature string
	config    *rest.Config
	transport http.RoundTripper
}

func isClusterLocalURL(urlStr string) bool {
	return strings.Contains(urlStr, ".svc.cluster.local") || strings.Contains(urlStr, ".svc:")
}

func createK8sProxyTransport(k8sConfig *rest.Config, transport http.RoundTripper, prometheusURL string) (*k8sProxyTransport, error) {
	parsedURL, err := url.Parse(prometheusURL)
	if err != nil {
		return nil, err
	}

	parts := strings.Split(parsedURL.Host, ".")
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid cluster local URL format")
	}
	svcName := parts[0]
	namespace := parts[1]

	if transport == nil {
		return nil, errors.New("kubernetes transport is required")
	}

	transportWrapper := &k8sProxyTransport{
		transport:    transport,
		apiServerURL: k8sConfig.Host,
		namespace:    namespace,
		svcName:      svcName,
		scheme:       parsedURL.Scheme,
	}
	transportWrapper.port = parsedURL.Port()
	if transportWrapper.port == "" {
		if parsedURL.Scheme == "https" {
			transportWrapper.port = "443"
		} else {
			transportWrapper.port = "80"
		}
	}

	return transportWrapper, nil
}

type k8sProxyTransport struct {
	transport    http.RoundTripper
	apiServerURL string
	namespace    string
	svcName      string
	scheme       string
	port         string
}

func (t *k8sProxyTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	proxyURL, err := url.Parse(t.apiServerURL)
	if err != nil {
		return nil, err
	}
	req.URL.Scheme = proxyURL.Scheme
	req.URL.Host = proxyURL.Host

	servicePath := fmt.Sprintf("/api/v1/namespaces/%s/services/%s:%s/proxy", t.namespace, t.svcName, t.port)
	req.URL.Path = servicePath + req.URL.Path

	return t.transport.RoundTrip(req)
}

func (cm *ClusterManager) GetClientSet(clusterName, idToken string) (*ClientSet, error) {
	if idToken == "" {
		return nil, errors.New("OIDC ID token is required")
	}
	if clusterName == "" {
		clusters, err := model.ListClusters()
		if err != nil {
			return nil, err
		}
		for _, candidate := range clusters {
			if candidate.Enable && (clusterName == "" || candidate.IsDefault) {
				clusterName = candidate.Name
				if candidate.IsDefault {
					break
				}
			}
		}
	}
	if clusterName == "" {
		return nil, errors.New("no clusters available")
	}
	cluster, err := model.GetClusterByName(clusterName)
	if err != nil || !cluster.Enable {
		return nil, fmt.Errorf("cluster not found: %s", clusterName)
	}
	runtime, err := cm.runtimeForCluster(cluster)
	if err != nil {
		return nil, err
	}
	httpClient := &http.Client{
		Transport: kubetransport.NewBearerAuthRoundTripper(idToken, runtime.transport),
	}
	return newUserClientSet(cluster.Name, runtime.config, httpClient, cluster.PrometheusURL)
}

func (cm *ClusterManager) runtimeForCluster(cluster *model.Cluster) (*clusterRuntime, error) {
	config, generation, err := cm.baseRESTConfig(cluster)
	if err != nil {
		return nil, err
	}
	kube.PrepareConfig(config)
	signature := clusterRuntimeSignature(cluster, config, generation)

	cm.runtimeMu.Lock()
	defer cm.runtimeMu.Unlock()
	if cm.runtimes == nil {
		cm.runtimes = make(map[uint]*clusterRuntime)
	}
	if existing := cm.runtimes[cluster.ID]; existing != nil && existing.signature == signature {
		return existing, nil
	}
	transportFor := cm.transportFor
	if transportFor == nil {
		transportFor = rest.TransportFor
	}
	transport, err := transportFor(config)
	if err != nil {
		return nil, fmt.Errorf("create shared Kubernetes transport: %w", err)
	}
	runtime := &clusterRuntime{
		signature: signature,
		config:    rest.CopyConfig(config),
		transport: transport,
	}
	if existing := cm.runtimes[cluster.ID]; existing != nil {
		closeIdleConnections(existing.transport)
	}
	cm.runtimes[cluster.ID] = runtime
	return runtime, nil
}

func (cm *ClusterManager) baseRESTConfig(cluster *model.Cluster) (*rest.Config, uint64, error) {
	var config *rest.Config
	var generation uint64
	if cluster.ClusterAgent || cluster.ConnectionMode == "tunnel" {
		var err error
		config, generation, err = cm.clusterAgentManager.RESTConfig(cluster.ID)
		if err != nil {
			return nil, 0, err
		}
	} else {
		if cluster.APIServerURL == "" {
			return nil, 0, errors.New("cluster API server URL is missing")
		}
		caData := []byte(cluster.CABundle)
		if cluster.CABundle != "" && !strings.Contains(cluster.CABundle, "BEGIN CERTIFICATE") {
			decoded, err := base64.StdEncoding.DecodeString(cluster.CABundle)
			if err != nil {
				return nil, 0, errors.New("cluster CA bundle must be PEM or base64-encoded PEM")
			}
			caData = decoded
		}
		config = &rest.Config{
			Host: cluster.APIServerURL,
			TLSClientConfig: rest.TLSClientConfig{
				CAData:     caData,
				ServerName: cluster.TLSServerName,
			},
		}
	}
	config = rest.CopyConfig(config)
	config.BearerToken = ""
	config.BearerTokenFile = ""
	config.Username = ""
	config.Password = ""
	config.CertData = nil
	config.KeyData = nil
	config.CertFile = ""
	config.KeyFile = ""
	return config, generation, nil
}

func clusterRuntimeSignature(cluster *model.Cluster, config *rest.Config, generation uint64) string {
	payload := fmt.Sprintf("%d\x00%s\x00%s\x00%s\x00%t\x00%d\x00%x",
		cluster.ID,
		cluster.ConnectionMode,
		config.Host,
		config.ServerName,
		config.Insecure,
		generation,
		config.CAData,
	)
	return fmt.Sprintf("%x", sha256.Sum256([]byte(payload)))
}

func closeIdleConnections(transport http.RoundTripper) {
	if closer, ok := transport.(interface{ CloseIdleConnections() }); ok {
		closer.CloseIdleConnections()
	}
}

func (cm *ClusterManager) invalidateRuntime(clusterID uint) {
	cm.runtimeMu.Lock()
	defer cm.runtimeMu.Unlock()
	runtime := cm.runtimes[clusterID]
	if runtime == nil {
		return
	}
	delete(cm.runtimes, clusterID)
	closeIdleConnections(runtime.transport)
}

func newUserClientSet(name string, config *rest.Config, httpClient *http.Client, prometheusURL string) (*ClientSet, error) {
	k8sClient, err := kube.NewDirectClient(config, httpClient)
	if err != nil {
		return nil, err
	}
	clientSet := &ClientSet{Name: name, K8sClient: k8sClient, prometheusURL: prometheusURL}
	if prometheusURL != "" && isClusterLocalURL(prometheusURL) {
		proxyTransport, err := createK8sProxyTransport(config, httpClient.Transport, prometheusURL)
		if err != nil {
			return nil, fmt.Errorf("create Kubernetes-authorized Prometheus transport: %w", err)
		}
		clientSet.PromClient, err = prometheus.NewClientWithRoundTripper(prometheusURL, proxyTransport)
		if err != nil {
			return nil, fmt.Errorf("create Prometheus client: %w", err)
		}
	}
	return clientSet, nil
}

func NewClusterManager() (*ClusterManager, error) {
	cm := &ClusterManager{
		runtimes:     make(map[uint]*clusterRuntime),
		transportFor: rest.TransportFor,
	}
	cm.clusterAgentManager = clusteragent.NewManager(func() {})
	return cm, nil
}
