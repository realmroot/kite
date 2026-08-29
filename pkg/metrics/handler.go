package metrics

import (
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/realmroot/lightkite/pkg/cluster"
	"github.com/realmroot/lightkite/pkg/kube"
	"github.com/realmroot/lightkite/pkg/prometheus"
	authorizationv1 "k8s.io/api/authorization/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
)

type Handler struct {
	metricsServerCache     map[string][]prometheus.UsageDataPoint
	metricsServerCacheLock sync.Mutex
	metricsServerMaxSeries int
}

const (
	defaultMetricsServerMaxSeries = 8192
	maxMetricsServerPoints        = 121
)

var validDurations = map[string]bool{
	"30m": true,
	"1h":  true,
	"24h": true,
}

func NewHandler() *Handler {
	return &Handler{
		metricsServerCache:     make(map[string][]prometheus.UsageDataPoint),
		metricsServerMaxSeries: defaultMetricsServerMaxSeries,
	}
}

func (h *Handler) GetResourceUsageHistory(c *gin.Context) {
	ctx := c.Request.Context()

	cs := c.MustGet("cluster").(*cluster.ClientSet)

	// Get query parameter for time range
	duration := c.DefaultQuery("duration", "1h")

	// Validate duration parameter
	if !validDurations[duration] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid duration. Must be one of: 30m, 1h, 24h"})
		return
	}

	// Get resource usage history if Prometheus is available
	if cs.PromClient == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Prometheus client not available"})
		return
	}

	instance := c.Query("instance")
	attributes := authorizationv1.ResourceAttributes{
		Verb:     "list",
		Resource: "nodes",
	}
	if instance != "" {
		attributes.Verb = "get"
		attributes.Name = instance
	}
	if !authorizePrometheusResource(c, cs, attributes) {
		return
	}
	resourceUsageHistory, err := cs.PromClient.GetResourceUsageHistory(ctx, instance, duration, "instance")
	if err != nil {
		resourceUsageHistory, err = cs.PromClient.GetResourceUsageHistory(ctx, instance, duration, "node")
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to get resource usage history: %v", err)})
			return
		}
	}

	c.JSON(http.StatusOK, resourceUsageHistory)
}

// GetPodMetrics handles pod-specific metrics requests
func (h *Handler) GetPodMetrics(c *gin.Context) {
	ctx := c.Request.Context()
	cs := c.MustGet("cluster").(*cluster.ClientSet)
	namespace := c.Param("namespace")

	// Get path parameters
	podName := c.Param("podName")
	if namespace == "" || podName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "namespace and podName are required"})
		return
	}

	// Get query parameters
	duration := c.DefaultQuery("duration", "1h")
	container := c.Query("container") // Optional container name
	labelSelector := c.Query("labelSelector")

	// Validate duration parameter
	if !validDurations[duration] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid duration. Must be one of: 30m, 1h, 24h"})
		return
	}

	// Try Prometheus first
	var podMetrics *prometheus.PodMetrics
	var err error
	if cs.PromClient != nil {
		if !authorizePrometheusResource(c, cs, authorizationv1.ResourceAttributes{
			Namespace: namespace,
			Verb:      "get",
			Resource:  "pods",
			Name:      podName,
		}) {
			return
		}
		podMetrics, err = cs.PromClient.GetPodMetrics(ctx, namespace, podName, container, duration)
		if err == nil && podMetrics != nil {
			podMetrics.Fallback = false
			c.JSON(http.StatusOK, podMetrics)
			return
		}
	}

	// Fallback: metrics-server
	podMetrics, err = h.fetchPodMetricsFromMetricsServer(c, namespace, podName, container, labelSelector)
	if err != nil {
		writeMetricsServerError(c, err)
		return
	}
	podMetrics.Fallback = true
	c.JSON(http.StatusOK, podMetrics)
}

func authorizePrometheusResource(c *gin.Context, cs *cluster.ClientSet, attributes authorizationv1.ResourceAttributes) bool {
	allowed, reason, err := kube.CheckSelfSubjectAccess(c.Request.Context(), cs.K8sClient.ClientSet, attributes)
	if err != nil {
		writeMetricsKubernetesError(c, err, "Failed to authorize Prometheus metrics")
		return false
	}
	if allowed {
		return true
	}
	if reason == "" {
		reason = "Kubernetes RBAC denied access to metrics for this resource"
	}
	writeMetricsKubernetesError(c, apierrors.NewForbidden(
		schema.GroupResource{Group: attributes.Group, Resource: attributes.Resource},
		attributes.Name,
		errors.New(reason),
	), "")
	return false
}

func writeMetricsKubernetesError(c *gin.Context, err error, prefix string) {
	status := http.StatusInternalServerError
	var apiStatus apierrors.APIStatus
	if errors.As(err, &apiStatus) {
		if code := int(apiStatus.Status().Code); code >= 400 && code <= 599 {
			status = code
		}
	}
	message := err.Error()
	if prefix != "" {
		message = fmt.Sprintf("%s: %s", prefix, message)
	}
	c.JSON(status, gin.H{"error": message})
}

func writeMetricsServerError(c *gin.Context, err error) {
	writeMetricsKubernetesError(c, err, "Failed to get pod metrics from both Prometheus and metrics-server")
}

func (h *Handler) fetchPodMetricsFromMetricsServer(c *gin.Context, namespace, podName, container, labelSelector string) (*prometheus.PodMetrics, error) {
	ctx := c.Request.Context()
	cs := c.MustGet("cluster").(*cluster.ClientSet)
	if cs.K8sClient.MetricsClient == nil {
		return nil, fmt.Errorf("metrics client not available")
	}

	if labelSelector != "" {
		listOpts := metav1.ListOptions{LabelSelector: labelSelector}
		podMetricsList, err := cs.K8sClient.MetricsClient.MetricsV1beta1().PodMetricses(namespace).List(ctx, listOpts)
		if err != nil {
			return nil, err
		}
		if len(podMetricsList.Items) == 0 {
			return nil, fmt.Errorf("no pod metrics found")
		}
		timestamp := time.Now()
		return h.recordMetricsServerSamples(metricsCacheClusterKey(cs), namespace, container, podMetricsList.Items, timestamp, true), nil
	}

	// single pod
	podMetrics, err := cs.K8sClient.MetricsClient.MetricsV1beta1().PodMetricses(namespace).Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return h.recordMetricsServerSamples(metricsCacheClusterKey(cs), namespace, container, []metricsv1beta1.PodMetrics{*podMetrics}, podMetrics.Timestamp.Time, false), nil
}

func metricsCacheClusterKey(cs *cluster.ClientSet) string {
	if cs.ClusterID != 0 {
		return "id:" + strconv.FormatUint(uint64(cs.ClusterID), 10)
	}
	return "name:" + cs.Name
}

func (h *Handler) recordMetricsServerSamples(clusterName, namespace, container string, pods []metricsv1beta1.PodMetrics, timestamp time.Time, aggregate bool) *prometheus.PodMetrics {
	h.metricsServerCacheLock.Lock()
	defer h.metricsServerCacheLock.Unlock()

	cutoff := time.Now().Add(-30 * time.Minute)
	for key, points := range h.metricsServerCache {
		points = retainRecentPoints(points, cutoff)
		if len(points) == 0 {
			delete(h.metricsServerCache, key)
			continue
		}
		h.metricsServerCache[key] = points
	}

	var cpuSeries, memSeries []prometheus.UsageDataPoint
	for _, podMetrics := range pods {
		for _, containerMetrics := range podMetrics.Containers {
			key := clusterName + "/" + namespace + "/" + podMetrics.Name + "/" + containerMetrics.Name
			cpuCacheKey := key + "/cpu"
			memCacheKey := key + "/mem"
			h.ensureMetricsSeriesCapacity(cpuCacheKey, "")
			h.metricsServerCache[cpuCacheKey] = appendMetricPoint(
				h.metricsServerCache[cpuCacheKey],
				float64(containerMetrics.Usage.Cpu().MilliValue())/1000.0,
				timestamp,
			)
			h.ensureMetricsSeriesCapacity(memCacheKey, cpuCacheKey)
			h.metricsServerCache[memCacheKey] = appendMetricPoint(
				h.metricsServerCache[memCacheKey],
				float64(containerMetrics.Usage.Memory().Value())/1024.0/1024.0,
				timestamp,
			)
			if container == "" || containerMetrics.Name == container {
				cpuSeries = append(cpuSeries, h.metricsServerCache[cpuCacheKey]...)
				memSeries = append(memSeries, h.metricsServerCache[memCacheKey]...)
			}
		}
	}

	if aggregate {
		cpuSeries = mergeUsageDataPointsSum(cpuSeries)
		memSeries = mergeUsageDataPointsSum(memSeries)
	}
	return &prometheus.PodMetrics{CPU: cpuSeries, Memory: memSeries, Fallback: true}
}

func (h *Handler) ensureMetricsSeriesCapacity(key, protectedKey string) {
	if _, exists := h.metricsServerCache[key]; exists {
		return
	}
	limit := h.metricsServerMaxSeries
	if limit <= 0 {
		limit = defaultMetricsServerMaxSeries
	}
	if len(h.metricsServerCache) < limit {
		return
	}
	var oldestKey string
	var oldestTimestamp time.Time
	for candidate, points := range h.metricsServerCache {
		if candidate == protectedKey {
			continue
		}
		if len(points) == 0 {
			oldestKey = candidate
			break
		}
		lastTimestamp := points[len(points)-1].Timestamp
		if oldestKey == "" || lastTimestamp.Before(oldestTimestamp) {
			oldestKey = candidate
			oldestTimestamp = lastTimestamp
		}
	}
	if oldestKey != "" {
		delete(h.metricsServerCache, oldestKey)
	}
}

func retainRecentPoints(points []prometheus.UsageDataPoint, cutoff time.Time) []prometheus.UsageDataPoint {
	firstRecent := sort.Search(len(points), func(i int) bool {
		return points[i].Timestamp.After(cutoff)
	})
	return points[firstRecent:]
}

func appendMetricPoint(points []prometheus.UsageDataPoint, value float64, timestamp time.Time) []prometheus.UsageDataPoint {
	if len(points) > 0 && timestamp.Sub(points[len(points)-1].Timestamp) < 15*time.Second {
		points[len(points)-1] = prometheus.UsageDataPoint{Timestamp: timestamp, Value: value}
		return points
	}
	points = append(points, prometheus.UsageDataPoint{Timestamp: timestamp, Value: value})
	if len(points) > maxMetricsServerPoints {
		points = points[len(points)-maxMetricsServerPoints:]
	}
	return points
}

func mergeUsageDataPointsSum(points []prometheus.UsageDataPoint) []prometheus.UsageDataPoint {
	m := make(map[int64]float64)
	for _, pt := range points {
		ts := pt.Timestamp.Unix()
		m[ts] += pt.Value
	}
	var merged []prometheus.UsageDataPoint
	for ts, value := range m {
		merged = append(merged, prometheus.UsageDataPoint{
			Timestamp: time.Unix(ts, 0),
			Value:     value,
		})
	}
	sort.Slice(merged, func(i, j int) bool {
		return merged[i].Timestamp.Before(merged[j].Timestamp)
	})
	return merged
}
