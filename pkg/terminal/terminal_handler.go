package terminal

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/realmroot/lightkite/pkg/cluster"
	"github.com/realmroot/lightkite/pkg/kube"
	"github.com/realmroot/lightkite/pkg/wsutil"
	"k8s.io/klog/v2"
)

type TerminalHandler struct {
}

func NewTerminalHandler() *TerminalHandler {
	return &TerminalHandler{}
}

// HandleTerminalWebSocket handles WebSocket connections for terminal sessions
func (h *TerminalHandler) HandleTerminalWebSocket(c *gin.Context) {
	// Get cluster info from context
	cs := c.MustGet("cluster").(*cluster.ClientSet)

	// Get path parameters
	namespace := c.Param("namespace")
	podName := c.Param("podName")
	container := c.Query("container")

	if namespace == "" || podName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "namespace and podName are required"})
		return
	}

	wsutil.Serve(c.Writer, c.Request, func(ws *wsutil.Session) {
		session := kube.NewTerminalSession(cs.K8sClient, ws.Conn, namespace, podName, container)
		defer session.Close()

		if err := session.Start(ws.Context, "exec"); err != nil {
			klog.V(2).Infof("Terminal session ended: %v", err)
		}
	})
}
