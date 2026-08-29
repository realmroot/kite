package cluster

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/zxh326/kite/pkg/common"
	"github.com/zxh326/kite/pkg/model"
	"gorm.io/gorm"
)

type createClusterRequest struct {
	Name          string `json:"name" binding:"required"`
	Description   string `json:"description"`
	APIServerURL  string `json:"apiServerUrl"`
	CABundle      string `json:"caBundle"`
	TLSServerName string `json:"tlsServerName"`
	PrometheusURL string `json:"prometheusURL"`
	IsDefault     bool   `json:"isDefault"`
	Enabled       *bool  `json:"enabled"`
}

type updateClusterRequest struct {
	Name          string `json:"name"`
	Description   string `json:"description"`
	APIServerURL  string `json:"apiServerUrl"`
	CABundle      string `json:"caBundle"`
	TLSServerName string `json:"tlsServerName"`
	PrometheusURL string `json:"prometheusURL"`
	IsDefault     bool   `json:"isDefault"`
	Enabled       bool   `json:"enabled"`
}

func (cm *ClusterManager) GetClusters(c *gin.Context) {
	clusters, err := model.ListClusters()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	result := make([]common.ClusterInfo, 0, len(clusters))
	for _, cluster := range clusters {
		if !cluster.Enable {
			continue
		}
		result = append(result, common.ClusterInfo{
			Name:      cluster.Name,
			IsDefault: cluster.IsDefault,
		})
	}
	if cm.inventoryCatalog != nil {
		for _, item := range cm.inventoryCatalog.list() {
			result = append(result, common.ClusterInfo{Name: item.name, DisplayName: item.displayName})
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})
	c.JSON(http.StatusOK, result)
}

func (cm *ClusterManager) GetClusterList(c *gin.Context) {
	clusters, err := model.ListClusters()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	result := make([]gin.H, 0, len(clusters))
	for _, cluster := range clusters {
		clusterInfo := gin.H{
			"id":            cluster.ID,
			"name":          cluster.Name,
			"description":   cluster.Description,
			"apiServerUrl":  cluster.APIServerURL,
			"caBundle":      cluster.CABundle,
			"tlsServerName": cluster.TLSServerName,
			"enabled":       cluster.Enable,
			"isDefault":     cluster.IsDefault,
			"prometheusURL": cluster.PrometheusURL,
		}

		result = append(result, clusterInfo)
	}

	c.JSON(http.StatusOK, result)
}

func (cm *ClusterManager) CreateCluster(c *gin.Context) {
	if common.IsSectionManaged("clusters") {
		c.JSON(http.StatusForbidden, gin.H{"error": common.ManagedSectionError})
		return
	}

	var req createClusterRequest

	if err := bindClusterJSON(c, &req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.APIServerURL = strings.TrimSpace(req.APIServerURL)
	req.CABundle = strings.TrimSpace(req.CABundle)
	req.TLSServerName = strings.TrimSpace(req.TLSServerName)
	req.PrometheusURL = strings.TrimSpace(req.PrometheusURL)
	if err := ValidateDirectClusterMetadata(req.APIServerURL, req.CABundle, req.TLSServerName, req.PrometheusURL); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if _, err := model.GetClusterByName(req.Name); err == nil {
		c.JSON(http.StatusConflict, gin.H{"error": "cluster already exists"})
		return
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	cluster := &model.Cluster{
		Name:          req.Name,
		Description:   req.Description,
		APIServerURL:  req.APIServerURL,
		CABundle:      req.CABundle,
		TLSServerName: req.TLSServerName,
		PrometheusURL: req.PrometheusURL,
		IsDefault:     req.IsDefault,
		Enable:        enabled,
	}

	if err := model.AddCluster(cluster); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	result := gin.H{
		"id":      cluster.ID,
		"message": "cluster created successfully",
	}
	c.JSON(http.StatusCreated, result)
}

func (cm *ClusterManager) UpdateCluster(c *gin.Context) {
	if common.IsSectionManaged("clusters") {
		c.JSON(http.StatusForbidden, gin.H{"error": common.ManagedSectionError})
		return
	}

	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid cluster id"})
		return
	}

	var req updateClusterRequest

	if err := bindClusterJSON(c, &req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	req.PrometheusURL = strings.TrimSpace(req.PrometheusURL)
	if req.PrometheusURL != "" {
		if _, err := parseClusterLocalPrometheusURL(req.PrometheusURL); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	}

	cluster, err := model.GetClusterByID(uint(id))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "cluster not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.APIServerURL = strings.TrimSpace(req.APIServerURL)
	req.CABundle = strings.TrimSpace(req.CABundle)
	req.TLSServerName = strings.TrimSpace(req.TLSServerName)
	if err := ValidateDirectClusterMetadata(req.APIServerURL, req.CABundle, req.TLSServerName, req.PrometheusURL); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Name != "" && req.Name != cluster.Name {
		existing, lookupErr := model.GetClusterByName(req.Name)
		if lookupErr == nil && existing.ID != cluster.ID {
			c.JSON(http.StatusConflict, gin.H{"error": "cluster already exists"})
			return
		}
		if lookupErr != nil && !errors.Is(lookupErr, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusInternalServerError, gin.H{"error": lookupErr.Error()})
			return
		}
	}

	updates := map[string]interface{}{
		"description":     req.Description,
		"api_server_url":  req.APIServerURL,
		"ca_bundle":       req.CABundle,
		"tls_server_name": req.TLSServerName,
		"prometheus_url":  req.PrometheusURL,
		"is_default":      req.IsDefault,
		"enable":          req.Enabled,
	}

	if req.Name != "" && req.Name != cluster.Name {
		updates["name"] = req.Name
	}

	if err := model.UpdateCluster(cluster, updates); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	cm.invalidateRuntime(cluster.ID)

	c.JSON(http.StatusOK, gin.H{"message": "cluster updated successfully"})
}

func bindClusterJSON(c *gin.Context, target any) error {
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return errors.New("request body must contain one JSON object")
		}
		return err
	}
	return binding.Validator.ValidateStruct(target)
}

func (cm *ClusterManager) DeleteCluster(c *gin.Context) {
	if common.IsSectionManaged("clusters") {
		c.JSON(http.StatusForbidden, gin.H{"error": common.ManagedSectionError})
		return
	}

	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid cluster id"})
		return
	}

	cluster, err := model.GetClusterByID(uint(id))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "cluster not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}

	if cluster.IsDefault {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot delete default cluster"})
		return
	}

	if err := model.DeleteCluster(cluster); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	cm.invalidateRuntime(cluster.ID)
	c.JSON(http.StatusOK, gin.H{"message": "cluster deleted successfully"})
}
