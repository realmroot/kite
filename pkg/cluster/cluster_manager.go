package cluster

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/zxh326/kite/pkg/clusteragent"
	"github.com/zxh326/kite/pkg/kube"
	"github.com/zxh326/kite/pkg/model"
	"github.com/zxh326/kite/pkg/prometheus"
	"k8s.io/client-go/rest"
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
}

func isClusterLocalURL(urlStr string) bool {
	return strings.Contains(urlStr, ".svc.cluster.local") || strings.Contains(urlStr, ".svc:")
}

func createK8sProxyTransport(k8sConfig *rest.Config, prometheusURL string) (*k8sProxyTransport, error) {
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

	transport, err := rest.TransportFor(k8sConfig)
	if err != nil {
		return nil, err
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
		return nil, errors.New("realmroot ID token is required")
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
	config, err := cm.userRESTConfig(cluster, idToken)
	if err != nil {
		return nil, err
	}
	return newUserClientSet(cluster.Name, config, cluster.PrometheusURL)
}

func (cm *ClusterManager) userRESTConfig(cluster *model.Cluster, idToken string) (*rest.Config, error) {
	var config *rest.Config
	if cluster.ClusterAgent || cluster.ConnectionMode == "tunnel" {
		var err error
		config, _, err = cm.clusterAgentManager.RESTConfig(cluster.ID)
		if err != nil {
			return nil, err
		}
	} else {
		if cluster.APIServerURL == "" {
			return nil, errors.New("cluster API server URL is missing")
		}
		caData := []byte(cluster.CABundle)
		if cluster.CABundle != "" && !strings.Contains(cluster.CABundle, "BEGIN CERTIFICATE") {
			decoded, err := base64.StdEncoding.DecodeString(cluster.CABundle)
			if err != nil {
				return nil, errors.New("cluster CA bundle must be PEM or base64-encoded PEM")
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
	config.BearerToken = idToken
	config.BearerTokenFile = ""
	config.Username = ""
	config.Password = ""
	config.CertData = nil
	config.KeyData = nil
	config.CertFile = ""
	config.KeyFile = ""
	return config, nil
}

func newUserClientSet(name string, config *rest.Config, prometheusURL string) (*ClientSet, error) {
	k8sClient, err := kube.NewDirectClient(config)
	if err != nil {
		return nil, err
	}
	clientSet := &ClientSet{Name: name, K8sClient: k8sClient, prometheusURL: prometheusURL}
	if version, err := k8sClient.ClientSet.Discovery().ServerVersion(); err == nil {
		clientSet.Version = version.String()
	}
	if prometheusURL != "" {
		transport := http.DefaultTransport
		if isClusterLocalURL(prometheusURL) {
			if proxyTransport, err := createK8sProxyTransport(config, prometheusURL); err == nil {
				transport = proxyTransport
			}
		}
		clientSet.PromClient, _ = prometheus.NewClientWithRoundTripper(prometheusURL, transport)
	}
	return clientSet, nil
}

func NewClusterManager() (*ClusterManager, error) {
	cm := new(ClusterManager)
	cm.clusterAgentManager = clusteragent.NewManager(func() {})
	return cm, nil
}
