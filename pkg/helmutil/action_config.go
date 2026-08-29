package helmutil

import (
	"github.com/realmroot/lightkite/pkg/common"
	"helm.sh/helm/v4/pkg/action"
	"helm.sh/helm/v4/pkg/kube"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

type restClientGetter struct {
	config    *rest.Config
	namespace string
}

func init() {
	// Match Helm CLI's server-side apply field manager.
	kube.ManagedFieldsManager = "helm"
}

func NewActionConfig(config *rest.Config, namespace string) (*action.Configuration, error) {
	cfg := action.NewConfiguration()
	getter := &restClientGetter{config: config, namespace: namespace}
	if err := cfg.Init(getter, namespace, "secret"); err != nil {
		return nil, err
	}
	return cfg, nil
}

func StorageNamespace(namespace string) string {
	if namespace == common.AllNamespaces {
		return ""
	}
	return namespace
}

func (g *restClientGetter) ToRESTConfig() (*rest.Config, error) {
	return rest.CopyConfig(g.config), nil
}

func (g *restClientGetter) ToDiscoveryClient() (discovery.CachedDiscoveryInterface, error) {
	discoveryClient, err := discovery.NewDiscoveryClientForConfig(rest.CopyConfig(g.config))
	if err != nil {
		return nil, err
	}
	return memory.NewMemCacheClient(discoveryClient), nil
}

func (g *restClientGetter) ToRESTMapper() (meta.RESTMapper, error) {
	discoveryClient, err := g.ToDiscoveryClient()
	if err != nil {
		return nil, err
	}
	return restmapper.NewDeferredDiscoveryRESTMapper(discoveryClient), nil
}

func (g *restClientGetter) ToRawKubeConfigLoader() clientcmd.ClientConfig {
	return &restClientConfig{config: g.config, namespace: g.namespace}
}

type restClientConfig struct {
	config    *rest.Config
	namespace string
}

func (c *restClientConfig) RawConfig() (clientcmdapi.Config, error) {
	config := clientcmdapi.Config{
		Clusters: map[string]*clientcmdapi.Cluster{
			"lightkite": {
				Server:                   c.config.Host,
				CertificateAuthorityData: append([]byte(nil), c.config.CAData...),
				InsecureSkipTLSVerify:    c.config.Insecure,
				TLSServerName:            c.config.ServerName,
			},
		},
		AuthInfos: map[string]*clientcmdapi.AuthInfo{
			"lightkite": {Token: c.config.BearerToken},
		},
		Contexts: map[string]*clientcmdapi.Context{
			"lightkite": {
				Cluster:   "lightkite",
				AuthInfo:  "lightkite",
				Namespace: c.namespace,
			},
		},
		CurrentContext: "lightkite",
	}
	return config, nil
}

func (c *restClientConfig) ClientConfig() (*rest.Config, error) {
	return rest.CopyConfig(c.config), nil
}

func (c *restClientConfig) Namespace() (string, bool, error) {
	return c.namespace, true, nil
}

func (c *restClientConfig) ConfigAccess() clientcmd.ConfigAccess {
	return nil
}
