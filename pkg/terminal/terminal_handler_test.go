package terminal

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/zxh326/kite/pkg/cluster"
	"github.com/zxh326/kite/pkg/kube"
)

func TestPodTerminalValidatesTargetBeforeOpeningWebSocket(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/terminal", nil)
	c.Set("cluster", &cluster.ClientSet{Name: "prod", K8sClient: &kube.K8sClient{}})

	(&TerminalHandler{}).HandleTerminalWebSocket(c)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", recorder.Code)
	}
}
