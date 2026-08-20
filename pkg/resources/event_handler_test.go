package resources

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/zxh326/kite/pkg/cluster"
	"github.com/zxh326/kite/pkg/common"
	"github.com/zxh326/kite/pkg/kube"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestListResourceEventsUsesKubernetesAuthorization(t *testing.T) {
	gin.SetMode(gin.TestMode)
	forbidden := false
	eventServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if forbidden {
			w.WriteHeader(http.StatusForbidden)
			_ = json.NewEncoder(w).Encode(&metav1.Status{
				TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Status"},
				Status:   metav1.StatusFailure,
				Reason:   metav1.StatusReasonForbidden,
				Code:     http.StatusForbidden,
				Message:  "events is forbidden",
			})
			return
		}
		_ = json.NewEncoder(w).Encode(corev1.EventList{
			TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "EventList"},
			Items:    []corev1.Event{{ObjectMeta: metav1.ObjectMeta{Name: "scheduled", Namespace: "default"}}},
		})
	}))
	t.Cleanup(eventServer.Close)
	clientSet, err := kubernetes.NewForConfig(&rest.Config{Host: eventServer.URL})
	if err != nil {
		t.Fatal(err)
	}
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "target", Namespace: "default", UID: "pod-uid"}},
	).Build()
	cs := &cluster.ClientSet{Name: "prod", K8sClient: &kube.K8sClient{Client: k8sClient, ClientSet: clientSet}}

	originalHandlers := handlers
	handlers = map[string]resourceHandler{
		string(common.Pods): NewGenericResourceHandler[*corev1.Pod, *corev1.PodList](common.Pods),
	}
	t.Cleanup(func() { handlers = originalHandlers })
	router := gin.New()
	router.GET("/events", func(c *gin.Context) {
		c.Set("cluster", cs)
		NewEventHandler().ListResourceEvents(c)
	})

	request := func(path string) *httptest.ResponseRecorder {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		return response
	}

	response := request("/events?resource=pods&name=target&namespace=default")
	if response.Code != http.StatusOK {
		t.Fatalf("success status = %d: %s", response.Code, response.Body.String())
	}

	forbidden = true
	response = request("/events?resource=pods&name=target&namespace=default")
	if response.Code != http.StatusForbidden {
		t.Fatalf("forbidden status = %d, want %d: %s", response.Code, http.StatusForbidden, response.Body.String())
	}

	response = request("/events?resource=pods&name=missing&namespace=default")
	if response.Code != http.StatusNotFound {
		t.Fatalf("missing target status = %d, want %d: %s", response.Code, http.StatusNotFound, response.Body.String())
	}
}
