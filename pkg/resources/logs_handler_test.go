package resources

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/realmroot/lightkite/pkg/cluster"
	"github.com/realmroot/lightkite/pkg/kube"
	"github.com/realmroot/lightkite/pkg/wsutil"
)

func TestLogsWebSocketValidatesOptionsBeforeStreaming(t *testing.T) {
	gin.SetMode(gin.TestMode)
	clientSet := &cluster.ClientSet{Name: "prod", K8sClient: &kube.K8sClient{}}
	router := gin.New()
	router.GET("/logs/:namespace/:podName", func(c *gin.Context) {
		c.Set("cluster", clientSet)
		NewLogsHandler().HandleLogsWebSocket(c)
	})
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)

	tests := []struct {
		name        string
		query       string
		wantMessage string
	}{
		{name: "tail lines", query: "tailLines=invalid", wantMessage: "invalid tailLines parameter"},
		{name: "since seconds", query: "sinceSeconds=invalid", wantMessage: "invalid sinceSeconds parameter"},
		{name: "label selector", query: "labelSelector=%5Binvalid", wantMessage: "invalid labelSelector parameter"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			url := "ws" + strings.TrimPrefix(server.URL, "http") + "/logs/default/" + func() string {
				if test.name == "label selector" {
					return "_all"
				}
				return "web"
			}() + "?" + test.query
			conn, response, err := websocket.DefaultDialer.Dial(url, nil)
			if response != nil {
				defer func() { _ = response.Body.Close() }()
			}
			if err != nil {
				t.Fatalf("dial WebSocket: %v", err)
			}
			defer func() { _ = conn.Close() }()
			var message wsutil.Message
			if err := conn.ReadJSON(&message); err != nil {
				t.Fatalf("read error message: %v", err)
			}
			if message.Type != "error" || !strings.Contains(message.Data, test.wantMessage) {
				t.Fatalf("message = %#v", message)
			}
		})
	}
}
