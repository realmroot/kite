package kube

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/util/flowcontrol"

	metricsv1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
	metricsclient "k8s.io/metrics/pkg/client/clientset/versioned"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayapiv1 "sigs.k8s.io/gateway-api/apis/v1"
)

var runtimeScheme = runtime.NewScheme()

const (
	defaultKubeAPIQPS   = 50
	defaultKubeAPIBurst = 100
)

// kubeAPIQPS returns the QPS limit for the Kubernetes API client, overridable
// via the KITE_KUBE_API_QPS env var. Defaults to 50 (client-go default is 5,
// which is too low for large/remote clusters during the initial cache sync).
func kubeAPIQPS() float32 {
	if v := os.Getenv("KITE_KUBE_API_QPS"); v != "" {
		if qps, err := strconv.ParseFloat(v, 32); err == nil && qps > 0 {
			return float32(qps)
		}
	}
	return defaultKubeAPIQPS
}

// kubeAPIBurst returns the burst limit for the Kubernetes API client,
// overridable via the KITE_KUBE_API_BURST env var. Defaults to 100 (client-go
// default is 10).
func kubeAPIBurst() int {
	if v := os.Getenv("KITE_KUBE_API_BURST"); v != "" {
		if burst, err := strconv.Atoi(v); err == nil && burst > 0 {
			return burst
		}
	}
	return defaultKubeAPIBurst
}

func init() {
	_ = scheme.AddToScheme(runtimeScheme)
	_ = apiextensionsv1.AddToScheme(runtimeScheme)
	_ = gatewayapiv1.Install(runtimeScheme)
	_ = metricsv1.AddToScheme(runtimeScheme)
}

// K8sClient holds the Kubernetes client instances
type K8sClient struct {
	client.Client
	ClientSet     kubernetes.Interface
	Configuration *rest.Config
	MetricsClient *metricsclient.Clientset
	HTTPClient    *http.Client
}

func PrepareConfig(config *rest.Config) {
	if config.QPS == 0 {
		config.QPS = kubeAPIQPS()
	}
	if config.Burst == 0 {
		config.Burst = kubeAPIBurst()
	}
	if config.RateLimiter == nil {
		config.RateLimiter = flowcontrol.NewTokenBucketRateLimiter(config.QPS, config.Burst)
	}
}

// NewDirectClient creates lightweight clients over a caller-owned shared HTTP
// transport. It never starts an informer or owns the underlying connection pool.
func NewDirectClient(config *rest.Config, httpClient *http.Client) (*K8sClient, error) {
	if httpClient == nil {
		return nil, fmt.Errorf("kubernetes HTTP client is required")
	}
	config = rest.CopyConfig(config)
	PrepareConfig(config)
	clientset, err := kubernetes.NewForConfigAndClient(config, httpClient)
	if err != nil {
		return nil, err
	}
	metricsClient, err := metricsclient.NewForConfigAndClient(config, httpClient)
	if err != nil {
		return nil, fmt.Errorf("create metrics client: %w", err)
	}
	directClient, err := client.New(config, client.Options{Scheme: runtimeScheme, HTTPClient: httpClient})
	if err != nil {
		return nil, fmt.Errorf("create Kubernetes client: %w", err)
	}
	return &K8sClient{
		Client:        directClient,
		ClientSet:     clientset,
		Configuration: config,
		MetricsClient: metricsClient,
		HTTPClient:    httpClient,
	}, nil
}

// GetScheme returns the runtime scheme used by the client
func GetScheme() *runtime.Scheme {
	return runtimeScheme
}

func WaitForResourceDeletion(ctx context.Context, client client.Client, obj client.Object, timeout time.Duration) error {
	key := types.NamespacedName{
		Namespace: obj.GetNamespace(),
		Name:      obj.GetName(),
	}
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	timeoutCh := time.After(timeout)
	for {
		select {
		case <-timeoutCh:
			return fmt.Errorf("timed out waiting for resource deletion: %s", key)
		case <-ticker.C:
			if err := client.Get(ctx, key, obj); err != nil {
				if errors.IsNotFound(err) {
					return nil
				}
				return fmt.Errorf("failed to get resource: %w", err)
			} else if obj.GetDeletionTimestamp().IsZero() {
				// resource still exist, but deletion timestamp is not set
				// may be created again after deletion
				// we can consider it successfully deleted.
				return nil
			}
		}
	}
}
