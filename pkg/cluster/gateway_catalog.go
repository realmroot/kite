package cluster

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/gin-gonic/gin"
	"github.com/zxh326/kite/pkg/model"
	"gorm.io/gorm"
	"k8s.io/klog/v2"
)

const (
	gatewayCatalogSource = "cluster-gateway"
	gatewayAPIVersion    = "2026-08-27"
)

type gatewayCatalog struct {
	baseURL string
	client  *http.Client
}

type gatewayCluster struct {
	ID              string    `json:"id"`
	DisplayName     string    `json:"displayName"`
	Description     string    `json:"description"`
	APIServerURL    string    `json:"apiServerUrl"`
	CABundle        string    `json:"caBundle"`
	TLSServerName   string    `json:"tlsServerName"`
	PrometheusURL   string    `json:"prometheusUrl"`
	Enabled         bool      `json:"enabled"`
	Default         bool      `json:"default"`
	ResourceVersion uint64    `json:"resourceVersion"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

type gatewayClusterInput struct {
	DisplayName   string `json:"displayName"`
	Description   string `json:"description"`
	APIServerURL  string `json:"apiServerUrl"`
	CABundle      string `json:"caBundle"`
	TLSServerName string `json:"tlsServerName"`
	PrometheusURL string `json:"prometheusUrl"`
	Enabled       bool   `json:"enabled"`
	Default       bool   `json:"default"`
}

type gatewayClusterPage struct {
	Items      []gatewayCluster `json:"items"`
	Pagination struct {
		NextPageToken string `json:"nextPageToken"`
	} `json:"pagination"`
}

type GatewayAuditEvent struct {
	ID                uint64    `json:"id"`
	CreatedAt         time.Time `json:"createdAt"`
	RequestID         string    `json:"requestId"`
	TokenID           string    `json:"tokenId"`
	PrincipalType     string    `json:"principalType"`
	ControllerSubject string    `json:"controllerSubject"`
	AgentIssuer       string    `json:"agentIssuer"`
	AgentSubject      string    `json:"agentSubject"`
	UserSubject       string    `json:"userSubject"`
	ClientID          string    `json:"clientId"`
	Scopes            string    `json:"scopes"`
	ClusterID         string    `json:"clusterId"`
	Method            string    `json:"method"`
	Path              string    `json:"path"`
	Status            int       `json:"status"`
	DurationMillis    int64     `json:"durationMillis"`
}

type GatewayAuditPage struct {
	Items      []GatewayAuditEvent `json:"items"`
	Pagination struct {
		PageSize      int    `json:"pageSize"`
		NextPageToken string `json:"nextPageToken"`
	} `json:"pagination"`
}

func newGatewayCatalog(baseURL string) (*gatewayCatalog, error) {
	parsed, err := url.Parse(strings.TrimRight(baseURL, "/"))
	if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, errors.New("CLUSTER_GATEWAY_URL must be an absolute HTTP(S) URL without query or fragment")
	}
	return &gatewayCatalog{baseURL: strings.TrimRight(baseURL, "/"), client: &http.Client{Timeout: 30 * time.Second}}, nil
}

func (g *gatewayCatalog) List(ctx context.Context, token string) ([]gatewayCluster, error) {
	items := []gatewayCluster{}
	pageToken := ""
	for {
		query := url.Values{"pageSize": []string{"200"}}
		if pageToken != "" {
			query.Set("pageToken", pageToken)
		}
		var page gatewayClusterPage
		if err := g.request(ctx, token, http.MethodGet, "/api/catalog/clusters?"+query.Encode(), "", nil, &page); err != nil {
			return nil, err
		}
		items = append(items, page.Items...)
		if page.Pagination.NextPageToken == "" {
			return items, nil
		}
		pageToken = page.Pagination.NextPageToken
	}
}

func (g *gatewayCatalog) Get(ctx context.Context, token, id string) (*gatewayCluster, error) {
	var cluster gatewayCluster
	if err := g.request(ctx, token, http.MethodGet, "/api/catalog/clusters/"+url.PathEscape(id), "", nil, &cluster); err != nil {
		return nil, err
	}
	return &cluster, nil
}

func (g *gatewayCatalog) Put(ctx context.Context, token string, cluster gatewayCluster, create bool) (*gatewayCluster, error) {
	precondition := "*"
	if !create {
		precondition = `"` + strconv.FormatUint(cluster.ResourceVersion, 10) + `"`
	}
	input := gatewayClusterInput{
		DisplayName: cluster.DisplayName, Description: cluster.Description,
		APIServerURL: cluster.APIServerURL, CABundle: cluster.CABundle,
		TLSServerName: cluster.TLSServerName, PrometheusURL: cluster.PrometheusURL,
		Enabled: cluster.Enabled, Default: cluster.Default,
	}
	var stored gatewayCluster
	if err := g.request(ctx, token, http.MethodPut, "/api/catalog/clusters/"+url.PathEscape(cluster.ID), precondition, input, &stored); err != nil {
		return nil, err
	}
	return &stored, nil
}

func (g *gatewayCatalog) Delete(ctx context.Context, token, id string, resourceVersion uint64) error {
	return g.request(ctx, token, http.MethodDelete, "/api/catalog/clusters/"+url.PathEscape(id), `"`+strconv.FormatUint(resourceVersion, 10)+`"`, nil, nil)
}

func (g *gatewayCatalog) AuditEvents(ctx context.Context, token, pageToken string, pageSize int) (*GatewayAuditPage, error) {
	query := url.Values{"pageSize": []string{strconv.Itoa(pageSize)}}
	if pageToken != "" {
		query.Set("pageToken", pageToken)
	}
	var page GatewayAuditPage
	if err := g.request(ctx, token, http.MethodGet, "/api/catalog/audit-events?"+query.Encode(), "", nil, &page); err != nil {
		return nil, err
	}
	return &page, nil
}

func (g *gatewayCatalog) request(ctx context.Context, token, method, path, precondition string, body, response any) error {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, g.baseURL+path, reader)
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("API-Version", gatewayAPIVersion)
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if precondition == "*" {
		request.Header.Set("If-None-Match", precondition)
	} else if precondition != "" {
		request.Header.Set("If-Match", precondition)
	}
	result, err := g.client.Do(request)
	if err != nil {
		return fmt.Errorf("cluster gateway request: %w", err)
	}
	defer func() {
		if closeErr := result.Body.Close(); closeErr != nil {
			klog.V(4).Infof("cluster gateway: close response body: %v", closeErr)
		}
	}()
	klog.V(4).Infof("cluster gateway: method=%s path=%s status=%d", method, request.URL.EscapedPath(), result.StatusCode)
	if result.StatusCode < 200 || result.StatusCode >= 300 {
		message, _ := io.ReadAll(io.LimitReader(result.Body, 64<<10))
		return fmt.Errorf("cluster gateway returned %d: %s", result.StatusCode, strings.TrimSpace(string(message)))
	}
	if response != nil && result.StatusCode != http.StatusNoContent {
		if err := json.NewDecoder(result.Body).Decode(response); err != nil {
			return fmt.Errorf("decode cluster gateway response: %w", err)
		}
	}
	return nil
}

func (cm *ClusterManager) syncGatewayCatalog(ctx context.Context, token string) ([]*model.Cluster, error) {
	clusters, _, err := cm.syncGatewayCatalogWithDetails(ctx, token)
	return clusters, err
}

func (cm *ClusterManager) syncGatewayCatalogWithDetails(ctx context.Context, token string) ([]*model.Cluster, map[string]gatewayCluster, error) {
	remote, err := cm.gatewayCatalog.List(ctx, token)
	if err != nil {
		return nil, nil, err
	}
	result := make([]*model.Cluster, 0, len(remote))
	seen := make(map[string]struct{}, len(remote))
	details := make(map[string]gatewayCluster, len(remote))
	for i := range remote {
		cluster, err := cm.projectGatewayCluster(&remote[i])
		if err != nil {
			return nil, nil, err
		}
		seen[remote[i].ID] = struct{}{}
		details[remote[i].ID] = remote[i]
		result = append(result, cluster)
	}
	var projections []*model.Cluster
	if err := model.DB.Where("catalog_source = ?", gatewayCatalogSource).Find(&projections).Error; err != nil {
		return nil, nil, err
	}
	for _, projection := range projections {
		if _, ok := seen[projection.CatalogID]; !ok {
			cm.invalidateRuntime(projection.ID)
			if err := model.DeleteCluster(projection); err != nil {
				return nil, nil, err
			}
		}
	}
	return result, details, nil
}

func (cm *ClusterManager) projectGatewayCluster(remote *gatewayCluster) (*model.Cluster, error) {
	var cluster model.Cluster
	err := model.DB.Where("catalog_source = ? AND catalog_id = ?", gatewayCatalogSource, remote.ID).First(&cluster).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	create := errors.Is(err, gorm.ErrRecordNotFound)
	previousVersion := cluster.CatalogResourceVersion
	cluster.CatalogSource = gatewayCatalogSource
	cluster.CatalogID = remote.ID
	cluster.CatalogResourceVersion = remote.ResourceVersion
	cluster.Name = remote.DisplayName
	cluster.Description = remote.Description
	cluster.APIServerURL = cm.gatewayCatalog.baseURL + "/clusters/" + url.PathEscape(remote.ID) + "/kubernetes"
	cluster.CABundle = ""
	cluster.TLSServerName = ""
	cluster.ConnectionMode = "gateway"
	cluster.PrometheusURL = remote.PrometheusURL
	cluster.ClusterAgent = false
	cluster.Enable = remote.Enabled
	cluster.IsDefault = remote.Default
	if create {
		if err := model.DB.Create(&cluster).Error; err != nil {
			return nil, err
		}
	} else if err := model.DB.Save(&cluster).Error; err != nil {
		return nil, err
	}
	if !create && previousVersion != remote.ResourceVersion {
		cm.invalidateRuntime(cluster.ID)
	}
	return &cluster, nil
}

func (cm *ClusterManager) createGatewayCluster(c *gin.Context, req createClusterRequest) {
	if req.ConnectionMode != "direct" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cluster Gateway currently accepts direct cluster metadata; tunnel enrollment belongs to the Gateway"})
		return
	}
	if err := ValidateDirectClusterMetadata(req.APIServerURL, req.CABundle, req.TLSServerName, req.PrometheusURL); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	remote := gatewayCluster{
		ID: gatewayClusterID(req.Name), DisplayName: strings.TrimSpace(req.Name), Description: req.Description,
		APIServerURL: strings.TrimSpace(req.APIServerURL), CABundle: strings.TrimSpace(req.CABundle),
		TLSServerName: strings.TrimSpace(req.TLSServerName), PrometheusURL: strings.TrimSpace(req.PrometheusURL),
		Enabled: enabled, Default: req.IsDefault,
	}
	stored, err := cm.gatewayCatalog.Put(c.Request.Context(), c.GetString("oidc-id-token"), remote, true)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	projection, err := cm.projectGatewayCluster(stored)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"id": projection.ID, "message": "cluster created successfully"})
}

func (cm *ClusterManager) updateGatewayCluster(c *gin.Context, cluster *model.Cluster, req updateClusterRequest) {
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}
	if err := ValidateDirectClusterMetadata(req.APIServerURL, req.CABundle, req.TLSServerName, req.PrometheusURL); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	remote := gatewayCluster{
		ID: cluster.CatalogID, DisplayName: req.Name, Description: req.Description,
		APIServerURL: strings.TrimSpace(req.APIServerURL), CABundle: strings.TrimSpace(req.CABundle),
		TLSServerName: strings.TrimSpace(req.TLSServerName), PrometheusURL: strings.TrimSpace(req.PrometheusURL),
		Enabled: req.Enabled, Default: req.IsDefault, ResourceVersion: cluster.CatalogResourceVersion,
	}
	stored, err := cm.gatewayCatalog.Put(c.Request.Context(), c.GetString("oidc-id-token"), remote, false)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	if _, err := cm.projectGatewayCluster(stored); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "cluster updated successfully"})
}

func (cm *ClusterManager) deleteGatewayCluster(c *gin.Context, cluster *model.Cluster) {
	if err := cm.gatewayCatalog.Delete(c.Request.Context(), c.GetString("oidc-id-token"), cluster.CatalogID, cluster.CatalogResourceVersion); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	if err := model.DeleteCluster(cluster); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	cm.invalidateRuntime(cluster.ID)
	c.JSON(http.StatusOK, gin.H{"message": "cluster deleted successfully"})
}

func (cm *ClusterManager) GetGatewayAuditEvents(c *gin.Context) {
	if cm.gatewayCatalog == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Cluster Gateway is not configured"})
		return
	}
	pageSize, err := strconv.Atoi(c.DefaultQuery("pageSize", "20"))
	if err != nil || pageSize < 1 || pageSize > 100 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "pageSize must be between 1 and 100"})
		return
	}
	page, err := cm.gatewayCatalog.AuditEvents(c.Request.Context(), c.GetString("oidc-id-token"), c.Query("pageToken"), pageSize)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, page)
}

func gatewayClusterID(name string) string {
	var result strings.Builder
	lastHyphen := false
	for _, value := range strings.ToLower(strings.TrimSpace(name)) {
		if unicode.IsLetter(value) || unicode.IsDigit(value) {
			result.WriteRune(value)
			lastHyphen = false
		} else if !lastHyphen && result.Len() != 0 {
			result.WriteByte('-')
			lastHyphen = true
		}
		if result.Len() >= 63 {
			break
		}
	}
	id := strings.Trim(result.String(), "-")
	if id == "" {
		return "cluster"
	}
	return id
}
