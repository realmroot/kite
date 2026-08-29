package proxy

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/realmroot/lightkite/pkg/cluster"
	"github.com/realmroot/lightkite/pkg/kube"
	"github.com/realmroot/lightkite/pkg/model"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"k8s.io/client-go/rest"
	kubetransport "k8s.io/client-go/transport"
)

func TestSuppressCanceledProxyAbort(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	request := httptest.NewRequest(http.MethodGet, "/watch", nil).WithContext(ctx)

	func() {
		defer suppressCanceledProxyAbort(request)
		panic(http.ErrAbortHandler)
	}()
}

func TestSuppressCanceledProxyAbortPreservesUnexpectedPanics(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/watch", nil)
	deferred := func() (recovered any) {
		defer func() { recovered = recover() }()
		func() {
			defer suppressCanceledProxyAbort(request)
			panic(http.ErrAbortHandler)
		}()
		return nil
	}()
	deferredError, isError := deferred.(error)
	if !isError || !errors.Is(deferredError, http.ErrAbortHandler) {
		t.Fatalf("recovered = %#v, want http.ErrAbortHandler", deferred)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

type closeNotifyRecorder struct {
	*httptest.ResponseRecorder
}

func (recorder *closeNotifyRecorder) CloseNotify() <-chan bool {
	return make(chan bool)
}

func TestKubernetesAPIProxyUsesUserIdentityAndPreservesResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var upstreamRequest *http.Request
	baseTransport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		upstreamRequest = request.Clone(request.Context())
		return &http.Response{
			StatusCode: http.StatusForbidden,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"kind":"Status","reason":"Forbidden"}`)),
			Request:    request,
		}, nil
	})
	userTransport := kubetransport.NewBearerAuthRoundTripper("signed-in-user-token", baseTransport)
	clientSet := &cluster.ClientSet{K8sClient: &kube.K8sClient{
		Configuration: &rest.Config{Host: "https://kubernetes.example.test/base"},
		HTTPClient:    &http.Client{Transport: userTransport},
	}}

	recorder := &closeNotifyRecorder{ResponseRecorder: httptest.NewRecorder()}
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/kubernetes/apis/apps/v1/namespaces/default/deployments?limit=50", nil)
	c.Request.Header.Set("Authorization", "Bearer browser-session-token")
	c.Request.Header.Set("Cookie", "kite_session=secret")
	c.Params = gin.Params{{Key: "path", Value: "/apis/apps/v1/namespaces/default/deployments"}}
	c.Set("cluster", clientSet)

	NewKubernetesAPIHandler().Proxy(c)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want Kubernetes 403; body=%s", recorder.Code, recorder.Body.String())
	}
	if upstreamRequest == nil {
		t.Fatal("Kubernetes transport was not called")
	}
	if got := upstreamRequest.URL.String(); got != "https://kubernetes.example.test/base/apis/apps/v1/namespaces/default/deployments?limit=50" {
		t.Fatalf("upstream URL = %q", got)
	}
	if got := upstreamRequest.Header.Get("Authorization"); got != "Bearer signed-in-user-token" {
		t.Fatalf("upstream authorization = %q", got)
	}
	if got := upstreamRequest.Header.Get("Cookie"); got != "" {
		t.Fatalf("browser cookie leaked upstream: %q", got)
	}
	if !strings.Contains(recorder.Body.String(), `"reason":"Forbidden"`) {
		t.Fatalf("Kubernetes response body was not preserved: %s", recorder.Body.String())
	}
}

func TestKubernetesAPIProxyRejectsParentTraversal(t *testing.T) {
	gin.SetMode(gin.TestMode)
	called := false
	recorder := &closeNotifyRecorder{ResponseRecorder: httptest.NewRecorder()}
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/kubernetes/api/v1/../secrets", nil)
	c.Params = gin.Params{{Key: "path", Value: "/api/v1/../secrets"}}
	c.Set("cluster", &cluster.ClientSet{K8sClient: &kube.K8sClient{
		Configuration: &rest.Config{Host: "https://kubernetes.example.test"},
		HTTPClient: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			called = true
			return nil, nil
		})},
	}})

	NewKubernetesAPIHandler().Proxy(c)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", recorder.Code)
	}
	if called {
		t.Fatal("invalid path reached Kubernetes transport")
	}
}

func TestKubernetesAPIProxyRecordsMutationsAtTheGatewayBoundary(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.ResourceHistory{}); err != nil {
		t.Fatal(err)
	}
	previousDB := model.DB
	model.DB = db
	t.Cleanup(func() { model.DB = previousDB })
	user := model.User{Username: "agent@example.test", Issuer: "https://id.example.test", Sub: "agent"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}

	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		body := `{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"demo","namespace":"default"},"spec":{"replicas":1}}`
		if request.Method == http.MethodPatch {
			body = `{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"demo","namespace":"default"},"spec":{"replicas":2}}`
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(body)),
			Request:    request,
		}, nil
	})
	clientSet := &cluster.ClientSet{
		Name: "kind", ClusterID: 7,
		K8sClient: &kube.K8sClient{
			Configuration: &rest.Config{Host: "https://kubernetes.example.test"},
			HTTPClient:    &http.Client{Transport: transport},
		},
	}
	recorder := &closeNotifyRecorder{ResponseRecorder: httptest.NewRecorder()}
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPatch, "/api/v1/kubernetes/apis/apps/v1/namespaces/default/deployments/demo", strings.NewReader(`{"spec":{"replicas":2}}`))
	c.Params = gin.Params{{Key: "path", Value: "/apis/apps/v1/namespaces/default/deployments/demo"}}
	c.Set("cluster", clientSet)
	c.Set("user", user)

	NewKubernetesAPIHandler().Proxy(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", recorder.Code, recorder.Body.String())
	}
	var history model.ResourceHistory
	if err := db.First(&history).Error; err != nil {
		t.Fatal(err)
	}
	if history.ClusterID != 7 || history.ResourceType != "deployments" || history.Namespace != "default" || history.ResourceName != "demo" || history.OperationType != "update" || !history.Success {
		t.Fatalf("unexpected history: %#v", history)
	}
	if !strings.Contains(history.PreviousYAML, "replicas: 1") || !strings.Contains(history.ResourceYAML, "replicas: 2") {
		t.Fatalf("history did not preserve rollback values: %#v", history)
	}
}
