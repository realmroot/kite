package metrics

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/realmroot/lightkite/pkg/cluster"
	"github.com/realmroot/lightkite/pkg/kube"
	"github.com/realmroot/lightkite/pkg/model"
	"github.com/realmroot/lightkite/pkg/prometheus"
	authorizationv1 "k8s.io/api/authorization/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
	k8stesting "k8s.io/client-go/testing"
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
	metricsclient "k8s.io/metrics/pkg/client/clientset/versioned"
)

func TestMergeUsageDataPointsSum(t *testing.T) {
	base := time.Date(2026, 3, 27, 10, 0, 0, 0, time.UTC)
	points := []prometheus.UsageDataPoint{
		{Timestamp: base.Add(200 * time.Millisecond), Value: 0.3},
		{Timestamp: base.Add(1 * time.Second), Value: 0.4},
		{Timestamp: base, Value: 0.2},
	}

	got := mergeUsageDataPointsSum(points)

	if len(got) != 2 {
		t.Fatalf("mergeUsageDataPointsSum() len = %d, want 2", len(got))
	}
	if got[0].Timestamp.Unix() != base.Unix() {
		t.Fatalf("first timestamp = %d, want %d", got[0].Timestamp.Unix(), base.Unix())
	}
	if got[0].Value != 0.5 {
		t.Fatalf("first value = %v, want 0.5", got[0].Value)
	}
	if got[1].Timestamp.Unix() != base.Add(1*time.Second).Unix() {
		t.Fatalf("second timestamp = %d, want %d", got[1].Timestamp.Unix(), base.Add(1*time.Second).Unix())
	}
	if got[1].Value != 0.4 {
		t.Fatalf("second value = %v, want 0.4", got[1].Value)
	}
}

func TestMetricsServerCacheIsIsolatedByCluster(t *testing.T) {
	handler := &Handler{metricsServerCache: make(map[string][]prometheus.UsageDataPoint)}
	timestamp := time.Now().UTC()
	metrics := []metricsv1beta1.PodMetrics{{
		ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "default"},
		Containers: []metricsv1beta1.ContainerMetrics{{
			Name: "app",
			Usage: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("100m"),
				corev1.ResourceMemory: resource.MustParse("64Mi"),
			},
		}},
	}}

	first := handler.recordMetricsServerSamples("cluster-a", "default", "", metrics, timestamp, false)
	metrics[0].Containers[0].Usage[corev1.ResourceCPU] = resource.MustParse("900m")
	second := handler.recordMetricsServerSamples("cluster-b", "default", "", metrics, timestamp, false)

	if len(first.CPU) != 1 || first.CPU[0].Value != 0.1 {
		t.Fatalf("cluster-a CPU = %#v, want one 0.1 sample", first.CPU)
	}
	if len(second.CPU) != 1 || second.CPU[0].Value != 0.9 {
		t.Fatalf("cluster-b CPU = %#v, want one 0.9 sample", second.CPU)
	}
	if len(handler.metricsServerCache) != 4 {
		t.Fatalf("cache series = %d, want 4 cluster-specific series", len(handler.metricsServerCache))
	}
}

func TestMetricsCacheUsesStableClusterIdentity(t *testing.T) {
	if got := metricsCacheClusterKey(&cluster.ClientSet{ClusterID: 42, Name: "renamed"}); got != "id:42" {
		t.Fatalf("metricsCacheClusterKey() = %q, want id:42", got)
	}
	if got := metricsCacheClusterKey(&cluster.ClientSet{Name: "legacy-test"}); got != "name:legacy-test" {
		t.Fatalf("fallback metricsCacheClusterKey() = %q, want name:legacy-test", got)
	}
}

func TestRecordMetricsServerSamplesPrunesExpiredSeries(t *testing.T) {
	now := time.Now().UTC()
	handler := &Handler{metricsServerCache: map[string][]prometheus.UsageDataPoint{
		"stale/cpu": {{Timestamp: now.Add(-31 * time.Minute), Value: 1}},
	}}

	handler.recordMetricsServerSamples("cluster-a", "default", "", nil, now, false)

	if len(handler.metricsServerCache) != 0 {
		t.Fatalf("cache = %#v, want expired series removed", handler.metricsServerCache)
	}
}

func TestMetricsServerCacheHasHardSeriesAndPointBounds(t *testing.T) {
	handler := &Handler{
		metricsServerCache:     make(map[string][]prometheus.UsageDataPoint),
		metricsServerMaxSeries: 2,
	}
	metricsForPod := func(name string) []metricsv1beta1.PodMetrics {
		return []metricsv1beta1.PodMetrics{{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
			Containers: []metricsv1beta1.ContainerMetrics{{
				Name: "app",
				Usage: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("100m"),
					corev1.ResourceMemory: resource.MustParse("64Mi"),
				},
			}},
		}}
	}
	now := time.Now().UTC()
	handler.recordMetricsServerSamples("cluster", "default", "", metricsForPod("old"), now, false)
	handler.recordMetricsServerSamples("cluster", "default", "", metricsForPod("current"), now.Add(time.Minute), false)

	if len(handler.metricsServerCache) != 2 {
		t.Fatalf("cache series = %d, want hard limit 2", len(handler.metricsServerCache))
	}
	for _, suffix := range []string{"cpu", "mem"} {
		key := "cluster/default/current/app/" + suffix
		if _, exists := handler.metricsServerCache[key]; !exists {
			t.Fatalf("current series %q was evicted: %#v", key, handler.metricsServerCache)
		}
	}

	var points []prometheus.UsageDataPoint
	for index := 0; index < maxMetricsServerPoints+25; index++ {
		points = appendMetricPoint(points, float64(index), now.Add(time.Duration(index)*16*time.Second))
	}
	if len(points) != maxMetricsServerPoints {
		t.Fatalf("series points = %d, want hard limit %d", len(points), maxMetricsServerPoints)
	}
}

func TestGetPodMetricsDefersAuthorizationToKubernetes(t *testing.T) {
	originalGinMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() {
		gin.SetMode(originalGinMode)
	})

	var calls atomic.Int32
	metricsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if r.Header.Get("Authorization") == "Bearer denied" {
			w.WriteHeader(http.StatusForbidden)
			_, _ = fmt.Fprint(w, `{
				"kind":"Status",
				"apiVersion":"v1",
				"status":"Failure",
				"message":"pod metrics are forbidden",
				"reason":"Forbidden",
				"code":403
			}`)
			return
		}
		_, _ = fmt.Fprint(w, `{
			"kind":"PodMetrics",
			"apiVersion":"metrics.k8s.io/v1beta1",
			"metadata":{"name":"web","namespace":"default"},
			"timestamp":"2026-07-17T00:00:00Z",
			"window":"30s",
			"containers":[{"name":"app","usage":{"cpu":"100m","memory":"64Mi"}}]
		}`)
	}))
	t.Cleanup(metricsServer.Close)

	metricsClient, err := metricsclient.NewForConfig(&rest.Config{Host: metricsServer.URL})
	if err != nil {
		t.Fatalf("create metrics client: %v", err)
	}
	allowedClientSet := &cluster.ClientSet{
		Name: "prod",
		K8sClient: &kube.K8sClient{
			MetricsClient: metricsClient,
		},
	}
	deniedMetricsClient, err := metricsclient.NewForConfig(&rest.Config{Host: metricsServer.URL, BearerToken: "denied"})
	if err != nil {
		t.Fatalf("create denied metrics client: %v", err)
	}
	deniedClientSet := &cluster.ClientSet{
		Name: "prod",
		K8sClient: &kube.K8sClient{
			MetricsClient: deniedMetricsClient,
		},
	}

	tests := []struct {
		name       string
		clientSet  *cluster.ClientSet
		wantStatus int
		wantCalls  int32
	}{
		{
			name:       "returns Kubernetes metrics",
			clientSet:  allowedClientSet,
			wantStatus: http.StatusOK,
			wantCalls:  1,
		},
		{
			name:       "preserves Kubernetes forbidden status",
			clientSet:  deniedClientSet,
			wantStatus: http.StatusForbidden,
			wantCalls:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls.Store(0)
			handler := &Handler{metricsServerCache: make(map[string][]prometheus.UsageDataPoint)}
			router := gin.New()
			router.GET("/prometheus/pods/:namespace/:podName/metrics", func(c *gin.Context) {
				c.Set("cluster", tt.clientSet)
				c.Set("user", model.User{Username: "alice"})
				handler.GetPodMetrics(c)
			})

			request := httptest.NewRequest(http.MethodGet, "/prometheus/pods/default/web/metrics?duration=30m", nil)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)

			if response.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", response.Code, tt.wantStatus, response.Body.String())
			}
			if got := calls.Load(); got != tt.wantCalls {
				t.Fatalf("metrics API calls = %d, want %d", got, tt.wantCalls)
			}
		})
	}
}

func TestPrometheusMetricsRequireUnderlyingKubernetesResourceAccess(t *testing.T) {
	originalGinMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(originalGinMode) })

	tests := []struct {
		name           string
		path           string
		wantAttributes authorizationv1.ResourceAttributes
	}{
		{
			name: "pod metrics require get pod",
			path: "/prometheus/pods/default/web/metrics?duration=30m",
			wantAttributes: authorizationv1.ResourceAttributes{
				Namespace: "default",
				Verb:      "get",
				Resource:  "pods",
				Name:      "web",
			},
		},
		{
			name: "node metrics require get node",
			path: "/prometheus/resource-usage-history?duration=30m&instance=worker-1",
			wantAttributes: authorizationv1.ResourceAttributes{
				Verb:     "get",
				Resource: "nodes",
				Name:     "worker-1",
			},
		},
		{
			name: "cluster metrics require list nodes",
			path: "/prometheus/resource-usage-history?duration=30m",
			wantAttributes: authorizationv1.ResourceAttributes{
				Verb:     "list",
				Resource: "nodes",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clientSet := k8sfake.NewSimpleClientset()
			var gotAttributes authorizationv1.ResourceAttributes
			clientSet.PrependReactor("create", "selfsubjectaccessreviews", func(action k8stesting.Action) (bool, runtime.Object, error) {
				review := action.(k8stesting.CreateAction).GetObject().(*authorizationv1.SelfSubjectAccessReview)
				gotAttributes = *review.Spec.ResourceAttributes
				return true, &authorizationv1.SelfSubjectAccessReview{
					Status: authorizationv1.SubjectAccessReviewStatus{Allowed: false, Reason: "test denial"},
				}, nil
			})

			handler := NewHandler()
			router := gin.New()
			router.GET("/prometheus/pods/:namespace/:podName/metrics", func(c *gin.Context) {
				c.Set("cluster", &cluster.ClientSet{
					K8sClient:  &kube.K8sClient{ClientSet: clientSet},
					PromClient: &prometheus.Client{},
				})
				handler.GetPodMetrics(c)
			})
			router.GET("/prometheus/resource-usage-history", func(c *gin.Context) {
				c.Set("cluster", &cluster.ClientSet{
					K8sClient:  &kube.K8sClient{ClientSet: clientSet},
					PromClient: &prometheus.Client{},
				})
				handler.GetResourceUsageHistory(c)
			})

			request := httptest.NewRequest(http.MethodGet, tt.path, nil)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)

			if response.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want 403; body=%s", response.Code, response.Body.String())
			}
			if gotAttributes != tt.wantAttributes {
				t.Fatalf("attributes = %#v, want %#v", gotAttributes, tt.wantAttributes)
			}
		})
	}
}
