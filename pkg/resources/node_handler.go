package resources

import (
	"fmt"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/zxh326/kite/pkg/cluster"
	"github.com/zxh326/kite/pkg/common"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/kubectl/pkg/drain"
)

type NodeHandler struct {
	*GenericResourceHandler[*corev1.Node, *corev1.NodeList]
}

func NewNodeHandler() *NodeHandler {
	return &NodeHandler{GenericResourceHandler: NewGenericResourceHandler[*corev1.Node, *corev1.NodeList](common.Nodes)}
}

// DrainNode is retained because drain coordinates a cordon with a policy-aware
// sequence of Pod eviction requests; it is not one Kubernetes resource write.
func (h *NodeHandler) DrainNode(c *gin.Context) {
	nodeName := c.Param("name")
	ctx := c.Request.Context()
	cs := c.MustGet("cluster").(*cluster.ClientSet)
	var request struct {
		Force            bool `json:"force"`
		GracePeriod      int  `json:"gracePeriod" binding:"min=0"`
		DeleteLocal      bool `json:"deleteLocalData"`
		IgnoreDaemonsets bool `json:"ignoreDaemonsets"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body: " + err.Error()})
		return
	}
	var node corev1.Node
	if err := cs.K8sClient.Get(ctx, types.NamespacedName{Name: nodeName}, &node); err != nil {
		if errors.IsNotFound(err) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Node not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	drainer := &drain.Helper{
		Ctx: ctx, Client: cs.K8sClient.ClientSet, Force: request.Force,
		GracePeriodSeconds:  request.GracePeriod,
		IgnoreAllDaemonSets: request.IgnoreDaemonsets,
		DeleteEmptyDirData:  request.DeleteLocal,
		Out:                 io.Discard, ErrOut: io.Discard,
	}
	podDeleteList, errs := drainer.GetPodsForDeletion(nodeName)
	if len(errs) > 0 {
		message := ""
		for i, item := range errs {
			if i > 0 {
				message += "; "
			}
			message += item.Error()
		}
		c.JSON(http.StatusConflict, gin.H{"error": message})
		return
	}
	pods := podDeleteList.Pods()
	if err := drain.RunCordonOrUncordon(drainer, &node, false); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to cordon node: " + err.Error()})
		return
	}
	if err := drainer.DeleteOrEvictPods(pods); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to drain node: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"message": fmt.Sprintf("Node %s drained successfully", nodeName),
		"node":    node.Name, "pods": len(pods), "warnings": podDeleteList.Warnings(),
	})
}

func (h *NodeHandler) registerCustomRoutes(group *gin.RouterGroup) {
	group.PUT("/_all/:name/drain", h.DrainNode)
}
