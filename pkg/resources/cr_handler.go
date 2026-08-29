package resources

import (
	"context"
	"math"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/zxh326/kite/pkg/cluster"
	"github.com/zxh326/kite/pkg/common"
	"github.com/zxh326/kite/pkg/kube"
	"github.com/zxh326/kite/pkg/model"
	"gorm.io/gorm"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/kubectl/pkg/describe"
)

type CRHandler struct{}

func NewCRHandler() *CRHandler { return &CRHandler{} }

func (h *CRHandler) getCRDByName(ctx context.Context, client *kube.K8sClient, crdName string) (*apiextensionsv1.CustomResourceDefinition, error) {
	var crd apiextensionsv1.CustomResourceDefinition
	if err := client.Get(ctx, types.NamespacedName{Name: crdName}, &crd); err != nil {
		return nil, err
	}
	return &crd, nil
}

func (h *CRHandler) getGVRFromCRD(crd *apiextensionsv1.CustomResourceDefinition) schema.GroupVersionResource {
	version := ""
	for _, candidate := range crd.Spec.Versions {
		if candidate.Storage {
			version = candidate.Name
			break
		}
	}
	if version == "" {
		for _, candidate := range crd.Spec.Versions {
			if candidate.Served {
				version = candidate.Name
				break
			}
		}
	}
	return schema.GroupVersionResource{Group: crd.Spec.Group, Version: version, Resource: crd.Spec.Names.Plural}
}

func validateCRNamespace(c *gin.Context, crd *apiextensionsv1.CustomResourceDefinition) bool {
	if crd.Spec.Scope != apiextensionsv1.ClusterScoped {
		return true
	}
	namespace := c.Param("namespace")
	if namespace == "" || namespace == common.AllNamespaces {
		return true
	}
	c.JSON(http.StatusBadRequest, gin.H{"error": "cluster-scoped custom resources must use _all as namespace"})
	return false
}

func (h *CRHandler) ListHistory(c *gin.Context) {
	cs := c.MustGet("cluster").(*cluster.ClientSet)
	crdName, resourceName, namespace := c.Param("crd"), c.Param("name"), c.Param("namespace")
	if crdName == "" || resourceName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "CRD name and resource name are required"})
		return
	}
	crd, err := h.getCRDByName(c.Request.Context(), cs.K8sClient, crdName)
	if err != nil {
		if errors.IsNotFound(err) {
			c.JSON(http.StatusNotFound, gin.H{"error": "CustomResourceDefinition not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if crd.Spec.Scope == apiextensionsv1.NamespaceScoped && (namespace == "" || namespace == common.AllNamespaces) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "namespace is required for namespaced custom resources"})
		return
	}
	authorizationNamespace := namespace
	if crd.Spec.Scope == apiextensionsv1.ClusterScoped {
		authorizationNamespace = ""
	}
	if !authorizeResourceHistoryRead(c, cs, crd.Spec.Group, crd.Spec.Names.Plural, authorizationNamespace, resourceName) {
		return
	}
	page, pageSize, ok := parseHistoryPagination(c)
	if !ok {
		return
	}
	baseQuery := func() *gorm.DB {
		query := scopeResourceHistoryCluster(model.DB.Model(&model.ResourceHistory{}), cs).
			Where("resource_type = ? AND resource_name = ?", crd.Name, resourceName)
		if crd.Spec.Scope == apiextensionsv1.NamespaceScoped {
			return query.Where("namespace = ?", namespace)
		}
		return query.Where("namespace IN ?", []string{"", common.AllNamespaces})
	}
	var total int64
	if err := baseQuery().Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	history := []model.ResourceHistory{}
	if err := baseQuery().Preload("Operator", func(db *gorm.DB) *gorm.DB {
		return db.Select("id", "issuer", "sub", "username", "name", "avatar_url")
	}).Order("created_at DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&history).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	totalPages := int(math.Ceil(float64(total) / float64(pageSize)))
	c.JSON(http.StatusOK, gin.H{"data": history, "pagination": gin.H{
		"page": page, "pageSize": pageSize, "total": total, "totalPages": totalPages,
		"hasNextPage": page < totalPages, "hasPrevPage": page > 1,
	}})
}

func (h *CRHandler) Describe(c *gin.Context) {
	cs := c.MustGet("cluster").(*cluster.ClientSet)
	crd, err := h.getCRDByName(c.Request.Context(), cs.K8sClient, c.Param("crd"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if !validateCRNamespace(c, crd) {
		return
	}
	gvr := h.getGVRFromCRD(crd)
	mapping := &meta.RESTMapping{
		Resource:         gvr,
		GroupVersionKind: schema.GroupVersionKind{Group: gvr.Group, Version: gvr.Version, Kind: crd.Spec.Names.Kind},
		Scope:            meta.RESTScopeNamespace,
	}
	if crd.Spec.Scope == apiextensionsv1.ClusterScoped {
		mapping.Scope = meta.RESTScopeRoot
	}
	describer, ok := describe.GenericDescriberFor(mapping, cs.K8sClient.Configuration)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create describer"})
		return
	}
	out, err := describer.Describe(c.Param("namespace"), c.Param("name"), describe.DescriberSettings{ShowEvents: true})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"result": out})
}
