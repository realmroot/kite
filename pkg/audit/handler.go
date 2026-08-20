package audit

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/zxh326/kite/pkg/model"
	"gorm.io/gorm"
)

const maxAuditPageSize = 100

type auditLogOperator struct {
	Username string `json:"username"`
	Issuer   string `json:"issuer"`
	Sub      string `json:"sub"`
}

type auditLogEntry struct {
	ID              uint              `json:"id"`
	CreatedAt       time.Time         `json:"createdAt"`
	UpdatedAt       time.Time         `json:"updatedAt"`
	ClusterName     string            `json:"clusterName"`
	ResourceType    string            `json:"resourceType"`
	ResourceName    string            `json:"resourceName"`
	Namespace       string            `json:"namespace"`
	OperationType   string            `json:"operationType"`
	OperationSource string            `json:"operationSource"`
	Success         bool              `json:"success"`
	ErrorMessage    string            `json:"errorMessage"`
	OperatorID      uint              `json:"operatorId"`
	Operator        *auditLogOperator `json:"operator"`
}

func toAuditLogEntry(history model.ResourceHistory) auditLogEntry {
	entry := auditLogEntry{
		ID:              history.ID,
		CreatedAt:       history.CreatedAt,
		UpdatedAt:       history.UpdatedAt,
		ClusterName:     history.ClusterName,
		ResourceType:    history.ResourceType,
		ResourceName:    history.ResourceName,
		Namespace:       history.Namespace,
		OperationType:   history.OperationType,
		OperationSource: history.OperationSource,
		Success:         history.Success,
		ErrorMessage:    history.ErrorMessage,
		OperatorID:      history.OperatorID,
	}
	if history.Operator != nil {
		entry.Operator = &auditLogOperator{
			Username: history.Operator.Username,
			Issuer:   history.Operator.Issuer,
			Sub:      history.Operator.Sub,
		}
	}
	return entry
}

func ListAuditLogs(c *gin.Context) {
	page := 1
	size := 20

	if p := strings.TrimSpace(c.Query("page")); p != "" {
		if parsed, err := strconv.Atoi(p); err == nil && parsed > 0 {
			page = parsed
		} else {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid page parameter"})
			return
		}
	}
	if s := strings.TrimSpace(c.Query("size")); s != "" {
		if parsed, err := strconv.Atoi(s); err == nil && parsed > 0 && parsed <= maxAuditPageSize {
			size = parsed
		} else {
			c.JSON(http.StatusBadRequest, gin.H{"error": "size must be between 1 and 100"})
			return
		}
	}

	operator := strings.TrimSpace(c.Query("operator"))
	search := strings.TrimSpace(c.Query("search"))
	operation := strings.TrimSpace(c.Query("operation"))
	clusterName := strings.TrimSpace(c.Query("cluster"))
	resourceType := strings.TrimSpace(c.Query("resourceType"))
	resourceName := strings.TrimSpace(c.Query("resourceName"))
	namespace := strings.TrimSpace(c.Query("namespace"))

	query := model.DB.Model(&model.ResourceHistory{})
	if operator != "" {
		like := "%" + operator + "%"
		query = query.Where(`EXISTS (
			SELECT 1 FROM users
			WHERE users.id = resource_histories.operator_id
			AND (users.username LIKE ? OR users.sub LIKE ? OR users.issuer LIKE ?)
		)`, like, like, like)
	}
	if clusterName != "" {
		query = query.Where("cluster_name = ?", clusterName)
	}
	if resourceType != "" {
		query = query.Where("resource_type = ?", resourceType)
	}
	if resourceName != "" {
		query = query.Where("resource_name = ?", resourceName)
	}
	if namespace != "" {
		query = query.Where("namespace = ?", namespace)
	}
	if search != "" {
		like := "%" + search + "%"
		query = query.Where("resource_name LIKE ?", like)
	}
	if operation != "" {
		query = query.Where("operation_type = ?", operation)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	history := []model.ResourceHistory{}
	if err := query.Select(
		"id", "created_at", "updated_at", "cluster_name", "resource_type", "resource_name", "namespace",
		"operation_type", "operation_source", "success", "error_message", "operator_id",
	).Preload("Operator", func(db *gorm.DB) *gorm.DB {
		return db.Select("id", "issuer", "sub", "username", "name", "avatar_url")
	}).Order("created_at DESC").Offset((page - 1) * size).Limit(size).Find(&history).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	entries := make([]auditLogEntry, 0, len(history))
	for _, item := range history {
		entries = append(entries, toAuditLogEntry(item))
	}

	c.JSON(http.StatusOK, gin.H{
		"data":  entries,
		"total": total,
		"page":  page,
		"size":  size,
	})
}
