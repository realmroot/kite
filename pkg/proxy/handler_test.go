package proxy

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/zxh326/kite/pkg/cluster"
)

func TestHandleProxyRejectsInvalidKind(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := &ProxyHandler{}
	router := gin.New()
	router.GET("/namespaces/:namespace/:kind/:name/proxy/*path", func(c *gin.Context) {
		c.Set("cluster", &cluster.ClientSet{Name: "prod"})
		handler.HandleProxy(c)
	})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/namespaces/default/deployments/web/proxy/health", nil)
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
}
