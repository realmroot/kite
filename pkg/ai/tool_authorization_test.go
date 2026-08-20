package ai

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/zxh326/kite/pkg/cluster"
	"github.com/zxh326/kite/pkg/model"
)

func TestAuthorizeToolRequiresAuthenticatedRequestAndCluster(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("missing context", func(t *testing.T) {
		if _, isError := AuthorizeTool(nil, &cluster.ClientSet{}, "get_resource", nil); !isError {
			t.Fatal("expected missing context to fail")
		}
	})

	t.Run("missing cluster", func(t *testing.T) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/ai/chat", nil)
		if _, isError := AuthorizeTool(c, nil, "get_resource", nil); !isError {
			t.Fatal("expected missing cluster to fail")
		}
	})

	t.Run("missing user", func(t *testing.T) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/ai/chat", nil)
		if _, isError := AuthorizeTool(c, &cluster.ClientSet{}, "get_resource", nil); !isError {
			t.Fatal("expected missing user to fail")
		}
	})
}

func TestAuthorizeToolDefersResourceAuthorizationToKubernetes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/ai/chat", nil)
	c.Set("user", model.User{Username: "alice"})

	result, isError := AuthorizeTool(c, &cluster.ClientSet{Name: "cluster-a"}, "delete_resource", map[string]interface{}{
		"kind": "Namespace",
		"name": "production",
	})
	if isError || result != "" {
		t.Fatalf("application authorization must defer to Kubernetes: result=%q error=%v", result, isError)
	}
}
