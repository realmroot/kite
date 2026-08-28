package cluster

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/zxh326/kite/pkg/clusteragent"
	"github.com/zxh326/kite/pkg/common"
	"github.com/zxh326/kite/pkg/model"
	"gorm.io/gorm"
)

type createClusterRequest struct {
	Name           string `json:"name" binding:"required"`
	Description    string `json:"description"`
	APIServerURL   string `json:"apiServerUrl"`
	CABundle       string `json:"caBundle"`
	TLSServerName  string `json:"tlsServerName"`
	ConnectionMode string `json:"connectionMode" binding:"required"`
	PrometheusURL  string `json:"prometheusURL"`
	IsDefault      bool   `json:"isDefault"`
	Enabled        *bool  `json:"enabled"`
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

func clusterAgentServerURL() string {
	return fmt.Sprintf("%s%s", strings.TrimRight(common.Host, "/"), common.Base)
}

func (cm *ClusterManager) GetClusters(c *gin.Context) {
	if cm.gatewayCatalog != nil {
		if _, err := cm.syncGatewayCatalog(c.Request.Context(), c.GetString("oidc-id-token")); err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
			return
		}
	}
	clusters, err := model.ListClusters()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	result := make([]common.ClusterInfo, 0, len(clusters))
	for _, cluster := range clusters {
		if cm.gatewayCatalog != nil && cluster.CatalogSource != gatewayCatalogSource {
			continue
		}
		if !cluster.Enable {
			continue
		}
		result = append(result, common.ClusterInfo{
			Name:      cluster.Name,
			IsDefault: cluster.IsDefault,
		})
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})
	c.JSON(http.StatusOK, result)
}

func (cm *ClusterManager) GetClusterList(c *gin.Context) {
	var gatewayDetails map[string]gatewayCluster
	if cm.gatewayCatalog != nil {
		var err error
		_, gatewayDetails, err = cm.syncGatewayCatalogWithDetails(c.Request.Context(), c.GetString("oidc-id-token"))
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
			return
		}
	}
	clusters, err := model.ListClusters()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	result := make([]gin.H, 0, len(clusters))
	for _, cluster := range clusters {
		if cm.gatewayCatalog != nil && cluster.CatalogSource != gatewayCatalogSource {
			continue
		}
		connectionMode := cluster.ConnectionMode
		apiServerURL := cluster.APIServerURL
		caBundle := cluster.CABundle
		tlsServerName := cluster.TLSServerName
		prometheusURL := cluster.PrometheusURL
		if cluster.CatalogSource == gatewayCatalogSource {
			connectionMode = "direct"
			remote := gatewayDetails[cluster.CatalogID]
			apiServerURL = remote.APIServerURL
			caBundle = remote.CABundle
			tlsServerName = remote.TLSServerName
			prometheusURL = remote.PrometheusURL
		}
		clusterInfo := gin.H{
			"id":             cluster.ID,
			"name":           cluster.Name,
			"description":    cluster.Description,
			"apiServerUrl":   apiServerURL,
			"caBundle":       caBundle,
			"tlsServerName":  tlsServerName,
			"connectionMode": connectionMode,
			"enabled":        cluster.Enable,
			"inCluster":      cluster.InCluster,
			"clusterAgent":   cluster.ClusterAgent,
			"connected":      cluster.ClusterAgent && cm.clusterAgentManager.Connected(cluster.ID),
			"isDefault":      cluster.IsDefault,
			"prometheusURL":  prometheusURL,
		}
		if cluster.ClusterAgent {
			clusterInfo["clusterAgentVersion"] = cm.clusterAgentManager.Version(cluster.ID)
		}

		result = append(result, clusterInfo)
	}

	c.JSON(http.StatusOK, result)
}

func (cm *ClusterManager) CreateCluster(c *gin.Context) {
	if cm.gatewayCatalog == nil && common.IsSectionManaged("clusters") {
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
	if cm.gatewayCatalog != nil {
		cm.createGatewayCluster(c, req)
		return
	}
	if req.ConnectionMode != "direct" && req.ConnectionMode != "tunnel" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "connectionMode must be direct or tunnel"})
		return
	}
	if req.ConnectionMode == "direct" {
		if err := ValidateDirectClusterMetadata(req.APIServerURL, req.CABundle, req.TLSServerName, req.PrometheusURL); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	} else if req.PrometheusURL != "" {
		if _, err := parseClusterLocalPrometheusURL(req.PrometheusURL); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	}

	if _, err := model.GetClusterByName(req.Name); err == nil {
		c.JSON(http.StatusConflict, gin.H{"error": "cluster already exists"})
		return
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var clusterAgentToken string
	var clusterAgentTokenHash string
	var clusterAgentPublicKey string
	var clusterAgentPrivateKey string
	var clusterAgentManifestGrant string
	if req.ConnectionMode == "tunnel" {
		var err error
		clusterAgentToken, clusterAgentTokenHash, err = clusteragent.NewToken()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		clusterAgentPublicKey, clusterAgentPrivateKey, err = clusteragent.NewRegistrationKeyPair()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		clusterAgentManifestGrant, err = cm.clusterAgentManager.CreateManifestGrant(clusterAgentToken)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	cluster := &model.Cluster{
		Name:                   req.Name,
		Description:            req.Description,
		APIServerURL:           req.APIServerURL,
		CABundle:               req.CABundle,
		TLSServerName:          req.TLSServerName,
		ConnectionMode:         req.ConnectionMode,
		PrometheusURL:          req.PrometheusURL,
		ClusterAgent:           req.ConnectionMode == "tunnel",
		ClusterAgentTokenHash:  clusterAgentTokenHash,
		ClusterAgentPublicKey:  clusterAgentPublicKey,
		ClusterAgentPrivateKey: model.SecretString(clusterAgentPrivateKey),
		IsDefault:              req.IsDefault,
		Enable:                 enabled,
	}

	if err := model.AddCluster(cluster); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	result := gin.H{
		"id":      cluster.ID,
		"message": "cluster created successfully",
	}
	if req.ConnectionMode == "tunnel" {
		serverURL := clusterAgentServerURL()
		result["clusterAgentServer"] = serverURL
		result["clusterAgentToken"] = clusterAgentToken
		result["clusterAgentPublicKey"] = clusterAgentPublicKey
		result["clusterAgentManifestURL"] = fmt.Sprintf("%s/api/v1/cluster-agent/manifest?grant=%s", strings.TrimRight(serverURL, "/"), clusterAgentManifestGrant)
	}
	c.JSON(http.StatusCreated, result)
}

func (cm *ClusterManager) UpdateCluster(c *gin.Context) {
	if cm.gatewayCatalog == nil && common.IsSectionManaged("clusters") {
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
	if cm.gatewayCatalog != nil {
		if cluster.CatalogSource != gatewayCatalogSource {
			c.JSON(http.StatusNotFound, gin.H{"error": "cluster not found"})
			return
		}
		cm.updateGatewayCluster(c, cluster, req)
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.APIServerURL = strings.TrimSpace(req.APIServerURL)
	req.CABundle = strings.TrimSpace(req.CABundle)
	req.TLSServerName = strings.TrimSpace(req.TLSServerName)
	if cluster.ConnectionMode == "direct" {
		if err := ValidateDirectClusterMetadata(req.APIServerURL, req.CABundle, req.TLSServerName, req.PrometheusURL); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
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
	if cm.gatewayCatalog == nil && common.IsSectionManaged("clusters") {
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
	if cm.gatewayCatalog != nil {
		if cluster.CatalogSource != gatewayCatalogSource {
			c.JSON(http.StatusNotFound, gin.H{"error": "cluster not found"})
			return
		}
		cm.deleteGatewayCluster(c, cluster)
		return
	}

	if err := model.DeleteCluster(cluster); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	cm.invalidateRuntime(cluster.ID)
	if cluster.ClusterAgent {
		cm.clusterAgentManager.Disconnect(cluster.ID)
	}

	c.JSON(http.StatusOK, gin.H{"message": "cluster deleted successfully"})
}

func (cm *ClusterManager) ConnectClusterAgent(c *gin.Context) {
	cm.clusterAgentManager.ServeHTTP(c.Writer, c.Request)
}

func (cm *ClusterManager) RegisterClusterAgent(c *gin.Context) {
	cm.clusterAgentManager.Register(c.Writer, c.Request)
}

func (cm *ClusterManager) GetClusterAgentManifest(c *gin.Context) {
	grant := strings.TrimSpace(c.Query("grant"))
	token, err := cm.clusterAgentManager.ResolveManifestGrant(grant)
	if errors.Is(err, clusteragent.ErrInvalidManifestGrant) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired manifest grant"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to validate manifest grant"})
		return
	}
	if token == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired manifest grant"})
		return
	}
	publicKey, err := cm.clusterAgentManager.RegistrationPublicKey(token)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load cluster agent registration key"})
		return
	}
	if publicKey == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired manifest grant"})
		return
	}
	serverURL := clusterAgentServerURL()
	image := model.DefaultGeneralClusterAgentImageValue()
	if setting, err := model.GetGeneralSetting(); err == nil && setting != nil && setting.ClusterAgentImage != "" {
		image = setting.ClusterAgentImage
	}
	if strings.TrimSpace(image) == "" {
		c.JSON(http.StatusConflict, gin.H{"error": "cluster agent image is not configured"})
		return
	}
	manifest := clusteragent.GenerateManifest(serverURL, token, publicKey, image)
	c.Header("Cache-Control", "no-store")
	c.Header("Content-Disposition", `attachment; filename="kite-cluster-agent.yaml"`)
	c.Data(http.StatusOK, "text/yaml; charset=utf-8", []byte(manifest))
}
