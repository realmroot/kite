package cluster

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/realmroot/lightkite/pkg/common"
	"github.com/realmroot/lightkite/pkg/kube"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/transport"
	"k8s.io/klog/v2"
	apisv1alpha1 "sigs.k8s.io/cluster-inventory-api/apis/v1alpha1"
	ciaclient "sigs.k8s.io/cluster-inventory-api/client/clientset/versioned"
	ciainformers "sigs.k8s.io/cluster-inventory-api/client/informers/externalversions"
)

const inventoryContextPrefix = "cluster-inventory--"

type inventoryCatalog struct {
	mu       sync.RWMutex
	contexts map[string]*inventoryContext
}

type inventoryContext struct {
	name        string
	displayName string
	config      *rest.Config
	transport   http.RoundTripper
}

func newInventoryCatalog(ctx context.Context) (*inventoryCatalog, error) {
	rootConfig, err := inventoryRootConfig()
	if err != nil {
		return nil, err
	}
	client, err := ciaclient.NewForConfig(rootConfig)
	if err != nil {
		return nil, fmt.Errorf("create Cluster Inventory client: %w", err)
	}
	factoryOptions := []ciainformers.SharedInformerOption{
		ciainformers.WithNamespace(common.ClusterInventoryNamespace),
	}
	if common.ClusterInventoryLabelSelector != "" {
		selector, err := labels.Parse(common.ClusterInventoryLabelSelector)
		if err != nil {
			return nil, fmt.Errorf("parse CLUSTER_INVENTORY_LABEL_SELECTOR: %w", err)
		}
		factoryOptions = append(factoryOptions, ciainformers.WithTweakListOptions(func(options *metav1.ListOptions) {
			options.LabelSelector = selector.String()
		}))
	}
	factory := ciainformers.NewSharedInformerFactoryWithOptions(client, 10*time.Minute, factoryOptions...)
	informer := factory.Apis().V1alpha1().ClusterProfiles().Informer()
	catalog := &inventoryCatalog{contexts: make(map[string]*inventoryContext)}
	if _, err := informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    func(obj any) { catalog.upsert(obj) },
		UpdateFunc: func(_, next any) { catalog.upsert(next) },
		DeleteFunc: func(obj any) { catalog.remove(obj) },
	}); err != nil {
		return nil, fmt.Errorf("register ClusterProfile event handler: %w", err)
	}
	factory.Start(ctx.Done())
	syncContext, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if !cache.WaitForCacheSync(syncContext.Done(), informer.HasSynced) {
		return nil, errors.New("cluster inventory cache did not synchronize within 15s")
	}
	return catalog, nil
}

func inventoryRootConfig() (*rest.Config, error) {
	if common.ClusterInventoryKubeconfig != "" {
		config, err := clientcmd.BuildConfigFromFlags("", common.ClusterInventoryKubeconfig)
		if err != nil {
			return nil, fmt.Errorf("load CLUSTER_INVENTORY_KUBECONFIG: %w", err)
		}
		return config, nil
	}
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("load in-cluster Cluster Inventory config: %w", err)
	}
	return config, nil
}

func (catalog *inventoryCatalog) upsert(obj any) {
	profile, ok := obj.(*apisv1alpha1.ClusterProfile)
	if !ok {
		klog.Warning("Cluster Inventory delivered a non-ClusterProfile object")
		return
	}
	context, err := inventoryContextFromProfile(profile)
	if err != nil {
		klog.Warningf("Ignoring ClusterProfile %s/%s: %v", profile.Namespace, profile.Name, err)
		catalog.delete(contextName(profile.Namespace, profile.Name))
		return
	}
	catalog.mu.Lock()
	previous := catalog.contexts[context.name]
	catalog.contexts[context.name] = context
	catalog.mu.Unlock()
	if previous != nil {
		closeIdleConnections(previous.transport)
	}
}

func (catalog *inventoryCatalog) remove(obj any) {
	profile, ok := obj.(*apisv1alpha1.ClusterProfile)
	if !ok {
		tombstone, tombstoneOK := obj.(cache.DeletedFinalStateUnknown)
		if tombstoneOK {
			profile, ok = tombstone.Obj.(*apisv1alpha1.ClusterProfile)
		}
	}
	if ok {
		catalog.delete(contextName(profile.Namespace, profile.Name))
	}
}

func (catalog *inventoryCatalog) delete(name string) {
	catalog.mu.Lock()
	previous := catalog.contexts[name]
	delete(catalog.contexts, name)
	catalog.mu.Unlock()
	if previous != nil {
		closeIdleConnections(previous.transport)
	}
}

func inventoryContextFromProfile(profile *apisv1alpha1.ClusterProfile) (*inventoryContext, error) {
	if len(profile.Status.AccessProviders) == 0 {
		return nil, errors.New("no access provider")
	}
	provider := profile.Status.AccessProviders[0]
	if strings.TrimSpace(provider.Cluster.Server) == "" {
		return nil, errors.New("access provider has no Kubernetes server")
	}
	config := &rest.Config{
		Host:               provider.Cluster.Server,
		DisableCompression: provider.Cluster.DisableCompression,
		TLSClientConfig: rest.TLSClientConfig{
			CAData:     append([]byte(nil), provider.Cluster.CertificateAuthorityData...),
			Insecure:   provider.Cluster.InsecureSkipTLSVerify,
			ServerName: provider.Cluster.TLSServerName,
		},
	}
	if provider.Cluster.CertificateAuthority != "" {
		return nil, errors.New("access provider must embed certificate authority data instead of a publisher-local file path")
	}
	if provider.Cluster.ProxyURL != "" {
		proxyURL, err := url.Parse(provider.Cluster.ProxyURL)
		if err != nil {
			return nil, fmt.Errorf("parse access provider proxy URL: %w", err)
		}
		config.Proxy = http.ProxyURL(proxyURL)
	}
	kube.PrepareConfig(config)
	roundTripper, err := rest.TransportFor(config)
	if err != nil {
		return nil, fmt.Errorf("create Kubernetes transport: %w", err)
	}
	displayName := strings.TrimSpace(profile.Spec.DisplayName)
	if displayName == "" {
		displayName = profile.Name
	}
	return &inventoryContext{
		name:        contextName(profile.Namespace, profile.Name),
		displayName: displayName,
		config:      config,
		transport:   roundTripper,
	}, nil
}

func contextName(namespace, name string) string {
	return inventoryContextPrefix + namespace + "--" + name
}

func (catalog *inventoryCatalog) list() []*inventoryContext {
	catalog.mu.RLock()
	result := make([]*inventoryContext, 0, len(catalog.contexts))
	for _, item := range catalog.contexts {
		result = append(result, item)
	}
	catalog.mu.RUnlock()
	sort.Slice(result, func(i, j int) bool { return result[i].name < result[j].name })
	return result
}

func (catalog *inventoryCatalog) get(name string) (*inventoryContext, bool) {
	catalog.mu.RLock()
	defer catalog.mu.RUnlock()
	item, ok := catalog.contexts[name]
	return item, ok
}

func (catalog *inventoryCatalog) clientSet(name, idToken string) (*ClientSet, error) {
	item, ok := catalog.get(name)
	if !ok {
		return nil, fmt.Errorf("cluster not found: %s", name)
	}
	httpClient := &http.Client{Transport: transport.NewBearerAuthRoundTripper(idToken, item.transport)}
	return newUserClientSet(item.name, item.config, httpClient, "", idToken)
}
