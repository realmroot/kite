package proxy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/zxh326/kite/pkg/cluster"
	"github.com/zxh326/kite/pkg/kube"
	"k8s.io/client-go/rest"
	kubetransport "k8s.io/client-go/transport"
)

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
