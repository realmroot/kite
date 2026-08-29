package resources

import (
	"math"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/realmroot/lightkite/pkg/cluster"
	"github.com/realmroot/lightkite/pkg/model"
	"gorm.io/gorm"
	"k8s.io/kubectl/pkg/describe"
)

func (h *GenericResourceHandler[T, V]) registerCustomRoutes(group *gin.RouterGroup) {}

func (h *GenericResourceHandler[T, V]) ListHistory(c *gin.Context) {
	cs := c.MustGet("cluster").(*cluster.ClientSet)
	namespace := c.Param("namespace")
	resourceName := c.Param("name")
	if !authorizeResourceHistoryRead(c, cs, h.getGroupKind().Group, h.name, namespace, resourceName) {
		return
	}
	page, pageSize, ok := parseHistoryPagination(c)
	if !ok {
		return
	}

	var total int64
	query := scopeResourceHistoryCluster(model.DB.Model(&model.ResourceHistory{}), cs).
		Where("resource_type = ? AND resource_name = ? AND namespace = ?", h.name, resourceName, namespace)
	if err := query.Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	history := []model.ResourceHistory{}
	historyQuery := scopeResourceHistoryCluster(model.DB.Preload("Operator", func(db *gorm.DB) *gorm.DB {
		return db.Select("id", "issuer", "sub", "username", "name", "avatar_url")
	}), cs).Where("resource_type = ? AND resource_name = ? AND namespace = ?", h.name, resourceName, namespace)
	if err := historyQuery.Order("created_at DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&history).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	totalPages := int(math.Ceil(float64(total) / float64(pageSize)))
	hasNextPage := page < totalPages
	hasPrevPage := page > 1

	response := gin.H{
		"data": history,
		"pagination": gin.H{
			"page":        page,
			"pageSize":    pageSize,
			"total":       total,
			"totalPages":  totalPages,
			"hasNextPage": hasNextPage,
			"hasPrevPage": hasPrevPage,
		},
	}

	c.JSON(http.StatusOK, response)
}

func (h *GenericResourceHandler[T, V]) Describe(c *gin.Context) {
	cs := c.MustGet("cluster").(*cluster.ClientSet)
	gk := h.getGroupKind()
	describer, ok := describe.DescriberFor(gk, cs.K8sClient.Configuration)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "no describer found for this resource"})
		return
	}
	namespace := c.Param("namespace")
	name := c.Param("name")
	out, err := describer.Describe(namespace, name, describe.DescriberSettings{
		ShowEvents: true,
	})
	if err != nil {
		writeKubernetesError(c, err, "Failed to describe resource")
		return
	}
	c.JSON(http.StatusOK, gin.H{"result": out})
}
