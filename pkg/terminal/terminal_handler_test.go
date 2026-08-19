package terminal

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/zxh326/kite/pkg/cluster"
	"github.com/zxh326/kite/pkg/kube"
	"github.com/zxh326/kite/pkg/model"
	"github.com/zxh326/kite/pkg/wsutil"
)

func TestPodTerminalRejectsUnauthorizedUserBeforeClusterAccess(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/terminal/:namespace/:podName", func(c *gin.Context) {
		c.Set("cluster", &cluster.ClientSet{Name: "prod", K8sClient: &kube.K8sClient{}})
		c.Set("user", model.User{Username: "alice"})
		(&TerminalHandler{}).HandleTerminalWebSocket(c)
	})
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)

	url := "ws" + strings.TrimPrefix(server.URL, "http") + "/terminal/default/web"
	conn, response, err := websocket.DefaultDialer.Dial(url, nil)
	if response != nil {
		defer func() { _ = response.Body.Close() }()
	}
	if err != nil {
		t.Fatalf("dialing WebSocket: %v", err)
	}
	defer func() { _ = conn.Close() }()
	var message wsutil.Message
	if err := conn.ReadJSON(&message); err != nil {
		t.Fatalf("reading rejection: %v", err)
	}
	if message.Type != "error" || !strings.Contains(message.Data, "does not have permission to exec pods") {
		t.Fatalf("rejection message = %#v", message)
	}
}
