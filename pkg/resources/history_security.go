package resources

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/zxh326/kite/pkg/cluster"
	"github.com/zxh326/kite/pkg/common"
	"github.com/zxh326/kite/pkg/kube"
	authorizationv1 "k8s.io/api/authorization/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const maxHistoryPageSize = 100

func authorizeResourceHistoryRead(c *gin.Context, cs *cluster.ClientSet, group, resource, namespace, name string) bool {
	if namespace == common.AllNamespaces {
		namespace = ""
	}
	allowed, reason, err := kube.CheckSelfSubjectAccess(
		c.Request.Context(),
		cs.K8sClient.ClientSet,
		authorizationv1.ResourceAttributes{
			Namespace: namespace,
			Verb:      "get",
			Group:     group,
			Resource:  resource,
			Name:      name,
		},
	)
	if err != nil {
		writeKubernetesError(c, err, "Failed to authorize resource history")
		return false
	}
	if allowed {
		return true
	}
	if reason == "" {
		reason = "Kubernetes RBAC denied access to resource history"
	}
	writeKubernetesError(c, apierrors.NewForbidden(
		schema.GroupResource{Group: group, Resource: resource},
		name,
		errors.New(reason),
	), "")
	return false
}

func parseHistoryPagination(c *gin.Context) (page, pageSize int, ok bool) {
	pageSize, err := strconv.Atoi(c.DefaultQuery("pageSize", "10"))
	if err != nil || pageSize < 1 || pageSize > maxHistoryPageSize {
		c.JSON(http.StatusBadRequest, gin.H{"error": "pageSize must be between 1 and 100"})
		return 0, 0, false
	}
	page, err = strconv.Atoi(c.DefaultQuery("page", "1"))
	if err != nil || page < 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "page must be greater than zero"})
		return 0, 0, false
	}
	return page, pageSize, true
}
