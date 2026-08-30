package cluster

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/realmroot/lightkite/pkg/common"
	"github.com/realmroot/lightkite/pkg/kube"
	"github.com/realmroot/lightkite/pkg/model"
	"github.com/realmroot/lightkite/pkg/prometheus"
	utilvalidation "k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/client-go/rest"
	kubetransport "k8s.io/client-go/transport"
)

type ClientSet struct {
	ClusterID  uint
	Name       string
	Version    string // Kubernetes version
	K8sClient  *kube.K8sClient
	PromClient *prometheus.Client

	DiscoveredPrometheusURL string
	prometheusURL           string
}

type ClusterManager struct {
	inventoryCatalog *inventoryCatalog
	runtimeMu        sync.Mutex
	runtimes         map[uint]*clusterRuntime
	transportFor     func(*rest.Config) (http.RoundTripper, error)
}

type clusterRuntime struct {
	signature string
	config    *rest.Config
	transport http.RoundTripper
}

func validateKubernetesAPIServerURL(value string) error {
	apiURL, err := url.Parse(strings.TrimSpace(value))
	if err != nil || apiURL.Scheme != "https" || apiURL.Host == "" {
		return errors.New("API server URL must be an absolute HTTPS URL")
	}
	if apiURL.User != nil || apiURL.RawQuery != "" || apiURL.ForceQuery || apiURL.Fragment != "" {
		return errors.New("API server URL must not contain credentials, query, or fragment")
	}
	return nil
}

func ValidateDirectClusterMetadata(apiServerURL, caBundle, tlsServerName, prometheusURL string) error {
	if err := validateKubernetesAPIServerURL(apiServerURL); err != nil {
		return err
	}
	if _, err := kube.NormalizeCABundle(caBundle); err != nil {
		return err
	}
	if err := kube.ValidateTLSServerName(tlsServerName); err != nil {
		return err
	}
	if strings.TrimSpace(prometheusURL) != "" {
		if _, err := parseClusterLocalPrometheusURL(strings.TrimSpace(prometheusURL)); err != nil {
			return err
		}
	}
	return nil
}

func parseClusterLocalPrometheusURL(urlStr string) (*url.URL, error) {
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return nil, fmt.Errorf("parse prometheus URL: %w", err)
	}
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return nil, errors.New("prometheus URL scheme must be http or https")
	}
	if parsedURL.User != nil || parsedURL.Path != "" || parsedURL.RawQuery != "" || parsedURL.Fragment != "" {
		return nil, errors.New("prometheus URL must be a service base URL without credentials, path, query, or fragment")
	}
	parts := strings.Split(parsedURL.Hostname(), ".")
	validSuffix := len(parts) == 3 && parts[2] == "svc"
	validFQDN := len(parts) == 5 && parts[2] == "svc" && parts[3] == "cluster" && parts[4] == "local"
	if (!validSuffix && !validFQDN) || len(utilvalidation.IsDNS1123Label(parts[0])) > 0 || len(utilvalidation.IsDNS1123Label(parts[1])) > 0 {
		return nil, errors.New("prometheus URL must target <service>.<namespace>.svc or <service>.<namespace>.svc.cluster.local")
	}
	return parsedURL, nil
}

func isClusterLocalURL(urlStr string) bool {
	_, err := parseClusterLocalPrometheusURL(urlStr)
	return err == nil
}

func createK8sProxyTransport(k8sConfig *rest.Config, transport http.RoundTripper, prometheusURL string) (*k8sProxyTransport, error) {
	parsedURL, err := parseClusterLocalPrometheusURL(prometheusURL)
	if err != nil {
		return nil, err
	}

	parts := strings.Split(parsedURL.Hostname(), ".")
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
		if clusterName == "" && cm.inventoryCatalog != nil {
			contexts := cm.inventoryCatalog.list()
			if len(contexts) > 0 {
				clusterName = contexts[0].name
			}
		}
	}
	if clusterName == "" {
		return nil, errors.New("no clusters available")
	}
	if cm.inventoryCatalog != nil && strings.HasPrefix(clusterName, inventoryContextPrefix) {
		return cm.inventoryCatalog.clientSet(clusterName, idToken)
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
	clientSet, err := newUserClientSet(cluster.Name, runtime.config, httpClient, cluster.PrometheusURL, idToken)
	if err != nil {
		return nil, err
	}
	clientSet.ClusterID = cluster.ID
	return clientSet, nil
}

func (cm *ClusterManager) runtimeForCluster(cluster *model.Cluster) (*clusterRuntime, error) {
	config, err := cm.baseRESTConfig(cluster)
	if err != nil {
		return nil, err
	}
	kube.PrepareConfig(config)
	signature := clusterRuntimeSignature(cluster, config)

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

func (cm *ClusterManager) baseRESTConfig(cluster *model.Cluster) (*rest.Config, error) {
	if err := validateKubernetesAPIServerURL(cluster.APIServerURL); err != nil {
		return nil, fmt.Errorf("invalid cluster API server URL: %w", err)
	}
	caData, err := kube.NormalizeCABundle(cluster.CABundle)
	if err != nil {
		return nil, fmt.Errorf("invalid cluster CA bundle: %w", err)
	}
	if err := kube.ValidateTLSServerName(cluster.TLSServerName); err != nil {
		return nil, err
	}
	config := &rest.Config{
		Host: cluster.APIServerURL,
		TLSClientConfig: rest.TLSClientConfig{
			CAData:     caData,
			ServerName: cluster.TLSServerName,
		},
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
	return config, nil
}

func clusterRuntimeSignature(cluster *model.Cluster, config *rest.Config) string {
	payload := fmt.Sprintf("%d\x00%s\x00%s\x00%t\x00%x",
		cluster.ID,
		config.Host,
		config.ServerName,
		config.Insecure,
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

func (cm *ClusterManager) InvalidateCatalogRuntimes() {
	cm.runtimeMu.Lock()
	for clusterID, runtime := range cm.runtimes {
		delete(cm.runtimes, clusterID)
		closeIdleConnections(runtime.transport)
	}
	cm.runtimeMu.Unlock()
}

func newUserClientSet(name string, config *rest.Config, httpClient *http.Client, prometheusURL, idToken string) (*ClientSet, error) {
	k8sClient, err := kube.NewDirectClient(config, httpClient)
	if err != nil {
		return nil, err
	}
	// Helm constructs additional clients from this request-scoped config. Keep
	// the user's token here without ever placing it in the shared cluster runtime.
	k8sClient.Configuration.BearerToken = idToken
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

func NewClusterManagerWithContext(ctx context.Context) (*ClusterManager, error) {
	cm := &ClusterManager{
		runtimes:     make(map[uint]*clusterRuntime),
		transportFor: rest.TransportFor,
	}
	if common.ClusterInventoryEnabled {
		catalog, err := newInventoryCatalog(ctx)
		if err != nil {
			return nil, err
		}
		cm.inventoryCatalog = catalog
	}
	return cm, nil
}
