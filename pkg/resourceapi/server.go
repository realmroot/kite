package resourceapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/zxh326/kite/pkg/cluster"
	"github.com/zxh326/kite/pkg/middleware"
	"github.com/zxh326/kite/pkg/model"
	"github.com/zxh326/kite/pkg/proxy"
	"gorm.io/gorm"
	"k8s.io/klog/v2"
)

const (
	ScopeClustersRead    = "clusters:read"
	ScopeKubernetesRead  = "kubernetes:read"
	ScopeKubernetesWrite = "kubernetes:write"
)

var supportedScopes = []string{ScopeClustersRead, ScopeKubernetesRead, ScopeKubernetesWrite}

type Server struct {
	resource     *url.URL
	metadataPath string
	openAPIPath  string
	openAPIURL   string
	issuer       string
	verifier     *verifier
	clusters     *cluster.ClusterManager
	db           *gorm.DB
}

func New(ctx context.Context, resourceURL, issuer string, clients, algorithms []string, clusters *cluster.ClusterManager, db *gorm.DB) (*Server, error) {
	resource, err := url.Parse(resourceURL)
	if err != nil {
		return nil, fmt.Errorf("parse protected resource URL: %w", err)
	}
	if resource.RawQuery != "" || resource.Fragment != "" || strings.HasSuffix(resource.Path, "/") {
		return nil, fmt.Errorf("protected resource URL must not contain a query, fragment, or trailing slash")
	}
	resourcePath := resource.EscapedPath()
	metadataPath := "/.well-known/oauth-protected-resource" + resourcePath
	openAPIPath := path.Dir(resourcePath) + "/openapi.json"
	openAPIURL := resource.Scheme + "://" + resource.Host + openAPIPath
	accessVerifier, err := newVerifier(ctx, issuer, resourceURL, clients, algorithms, db)
	if err != nil {
		return nil, err
	}
	return &Server{
		resource: resource, metadataPath: metadataPath, openAPIPath: openAPIPath,
		openAPIURL: openAPIURL, issuer: issuer, verifier: accessVerifier, clusters: clusters, db: db,
	}, nil
}

func (s *Server) Register(engine *gin.Engine) {
	engine.GET(s.metadataPath, s.metadata)
	engine.GET(s.resource.EscapedPath(), s.serviceDescription)
	engine.GET(s.openAPIPath, s.openAPI)

	protected := engine.Group(s.resource.EscapedPath())
	protected.Use(s.authenticate(), s.audit())
	protected.GET("/clusters", s.requireScope(ScopeClustersRead), s.listClusters)
	protected.Any("/clusters/:cluster/kubernetes/*path", s.requireKubernetesScope(), middleware.ClusterMiddleware(s.clusters), proxy.NewKubernetesAPIHandler().Proxy)
}

func (s *Server) metadata(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"resource":                          s.resource.String(),
		"authorization_servers":             []string{s.issuer},
		"scopes_supported":                  supportedScopes,
		"dpop_bound_access_tokens_required": true,
		"dpop_signing_alg_values_supported": []string{"ES256"},
	})
}

func (s *Server) serviceDescription(c *gin.Context) {
	c.Header("Link", fmt.Sprintf(`<%s>; rel="service-desc"; type="application/vnd.oai.openapi+json"`, s.openAPIURL))
	c.JSON(http.StatusOK, gin.H{"resource": s.resource.String(), "serviceDescription": s.openAPIURL})
}

func (s *Server) openAPI(c *gin.Context) {
	c.Header("Content-Type", "application/vnd.oai.openapi+json")
	c.JSON(http.StatusOK, s.contract())
}

func (s *Server) authenticate() gin.HandlerFunc {
	return func(c *gin.Context) {
		target := s.resource.Scheme + "://" + s.resource.Host + c.Request.URL.EscapedPath()
		identity, err := s.verifier.verify(c.Request.Context(), c.GetHeader("Authorization"), c.GetHeader("DPoP"), c.Request.Method, target)
		if err != nil {
			writeProtocolError(c, err)
			c.Abort()
			return
		}
		c.Set(principalKey, identity)
		c.Set(middleware.KubernetesBearerTokenKey, identity.Token)
		c.Next()
	}
}

func (s *Server) requireScope(required string) gin.HandlerFunc {
	return func(c *gin.Context) {
		identity := c.MustGet(principalKey).(*principal)
		if _, ok := identity.Scopes[required]; !ok {
			c.JSON(http.StatusForbidden, gin.H{"error": "insufficient_scope", "error_description": "Required scope: " + required})
			c.Abort()
			return
		}
		c.Next()
	}
}

func (s *Server) requireKubernetesScope() gin.HandlerFunc {
	return func(c *gin.Context) {
		required := ScopeKubernetesWrite
		if c.Request.Method == http.MethodGet || c.Request.Method == http.MethodHead {
			required = ScopeKubernetesRead
		}
		s.requireScope(required)(c)
	}
}

func (s *Server) audit() gin.HandlerFunc {
	return func(c *gin.Context) {
		identity := c.MustGet(principalKey).(*principal)
		record := &model.ResourceAccessAudit{
			RequestID: identity.TokenID, ControllerSubject: identity.Subject,
			AgentIssuer: identity.Actor.Issuer, AgentSubject: identity.Actor.Subject,
			ClientID: identity.ClientID, Scopes: identity.ScopeString,
			ClusterName: c.Param("cluster"), Method: c.Request.Method, Path: c.Request.URL.EscapedPath(),
		}
		if err := s.db.WithContext(c.Request.Context()).Create(record).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "audit_unavailable"})
			c.Abort()
			return
		}
		c.Next()
		status := c.Writer.Status()
		if status == 0 {
			status = http.StatusOK
		}
		if err := s.db.WithContext(context.WithoutCancel(c.Request.Context())).Model(record).Update("status", status).Error; err != nil {
			klog.Errorf("Failed to finalize Resource access audit %d: %v", record.ID, err)
		}
	}
}

func (s *Server) listClusters(c *gin.Context) {
	clusters, err := model.ListClusters()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "cluster_catalog_unavailable"})
		return
	}
	type item struct {
		Name        string `json:"name"`
		Description string `json:"description,omitempty"`
		Enabled     bool   `json:"enabled"`
		Default     bool   `json:"default"`
		Mode        string `json:"connectionMode"`
	}
	result := make([]item, 0, len(clusters))
	for _, candidate := range clusters {
		result = append(result, item{
			Name: candidate.Name, Description: candidate.Description, Enabled: candidate.Enable,
			Default: candidate.IsDefault, Mode: candidate.ConnectionMode,
		})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	c.JSON(http.StatusOK, gin.H{"clusters": result})
}

func writeProtocolError(c *gin.Context, err error) {
	var protocol *protocolError
	if !errors.As(err, &protocol) {
		protocol = &protocolError{code: "server_error", message: "The resource rejected the request", status: 500}
	}
	if protocol.status == http.StatusUnauthorized {
		c.Header("WWW-Authenticate", fmt.Sprintf(`DPoP error="%s"`, protocol.code))
	}
	c.JSON(protocol.status, gin.H{"error": protocol.code, "error_description": protocol.message})
}

func (s *Server) contract() gin.H {
	readSecurity := []gin.H{{"oauth": []string{ScopeKubernetesRead}}}
	writeSecurity := []gin.H{{"oauth": []string{ScopeKubernetesWrite}}}
	return gin.H{
		"openapi": "3.1.0",
		"info":    gin.H{"title": "Kite Kubernetes Resource API", "version": "1.0.0", "description": "Operate configured Kubernetes clusters under delegated user authority."},
		"servers": []gin.H{{"url": s.resource.String()}},
		"components": gin.H{
			"securitySchemes": gin.H{"oauth": gin.H{
				"type": "openIdConnect", "openIdConnectUrl": s.issuer + "/.well-known/openid-configuration", "x-dpop-required": true,
			}},
			"schemas": gin.H{
				"Cluster": gin.H{"type": "object", "required": []string{"name", "enabled", "default", "connectionMode"}, "properties": gin.H{
					"name": gin.H{"type": "string"}, "description": gin.H{"type": "string"}, "enabled": gin.H{"type": "boolean"},
					"default": gin.H{"type": "boolean"}, "connectionMode": gin.H{"type": "string", "enum": []string{"direct", "tunnel"}},
				}},
			},
		},
		"paths": gin.H{
			"/clusters": gin.H{"get": gin.H{
				"operationId": "listClusters", "summary": "List configured Kubernetes clusters",
				"security": []gin.H{{"oauth": []string{ScopeClustersRead}}},
				"responses": gin.H{"200": gin.H{"description": "Cluster catalog", "content": gin.H{"application/json": gin.H{"schema": gin.H{
					"type": "object", "required": []string{"clusters"}, "properties": gin.H{"clusters": gin.H{"type": "array", "items": gin.H{"$ref": "#/components/schemas/Cluster"}}},
				}}}}},
			}},
			"/clusters/{cluster}/kubernetes/{path}": gin.H{
				"parameters": []gin.H{
					{"name": "cluster", "in": "path", "required": true, "schema": gin.H{"type": "string"}},
					{"name": "path", "in": "path", "required": true, "schema": gin.H{"type": "string"}},
				},
				"get":    kubernetesOperation("getKubernetesResource", "Read from the Kubernetes API", readSecurity, false),
				"post":   kubernetesOperation("createKubernetesResource", "Create through the Kubernetes API", writeSecurity, true),
				"put":    kubernetesOperation("replaceKubernetesResource", "Replace through the Kubernetes API", writeSecurity, true),
				"patch":  kubernetesOperation("patchKubernetesResource", "Patch through the Kubernetes API", writeSecurity, true),
				"delete": kubernetesOperation("deleteKubernetesResource", "Delete through the Kubernetes API", writeSecurity, false),
			},
		},
	}
}

func kubernetesOperation(id, summary string, security []gin.H, acceptsBody bool) gin.H {
	operation := gin.H{
		"operationId": id, "summary": summary, "security": security,
		"responses": gin.H{
			"200":     gin.H{"description": "Kubernetes API response", "content": gin.H{"application/json": gin.H{"schema": gin.H{}}}},
			"default": gin.H{"description": "Kubernetes or authorization error"},
		},
	}
	if acceptsBody {
		operation["requestBody"] = gin.H{
			"required": true,
			"content": gin.H{
				"application/json": gin.H{"schema": gin.H{"type": "object", "additionalProperties": true}},
			},
		}
	}
	return operation
}
