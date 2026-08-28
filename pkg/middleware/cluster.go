package middleware

import (
	"net/http"
	"net/url"

	"github.com/gin-gonic/gin"
	"github.com/zxh326/kite/pkg/cluster"
)

const (
	ClusterNameHeader        = "x-cluster-name"
	ClusterNameKey           = "cluster-name"
	K8sClientKey             = "k8s-client"
	PromClientKey            = "prom-client"
	KubernetesBearerTokenKey = "kubernetes-bearer-token"
)

type clusterClientSetProvider interface {
	GetClientSet(string, string, string) (*cluster.ClientSet, error)
}

// ClusterMiddleware selects a cluster from the path, header, or query and injects its clients into context.
func ClusterMiddleware(cm clusterClientSetProvider) gin.HandlerFunc {
	return func(c *gin.Context) {
		clusterName := c.Param("cluster")
		if clusterName == "" {
			clusterName = c.GetHeader(ClusterNameHeader)
			if clusterName != "" {
				if decoded, err := url.PathUnescape(clusterName); err == nil {
					clusterName = decoded
				}
			} else {
				if v, ok := c.GetQuery(ClusterNameHeader); ok {
					clusterName = v
				}
			}
		}
		credential := c.GetString(KubernetesBearerTokenKey)
		if credential == "" {
			credential = c.GetString("oidc-id-token")
		}
		if credential == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "OIDC ID token is missing"})
			c.Abort()
			return
		}
		cluster, err := cm.GetClientSet(clusterName, credential, c.GetString("oidc-access-token"))
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			c.Abort()
			return
		}
		c.Set("cluster", cluster)
		c.Set(ClusterNameKey, cluster.Name)
		c.Next()
	}
}
