package proxy

import (
	"context"
	"errors"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/zxh326/kite/pkg/cluster"
)

type KubernetesAPIHandler struct{}

func NewKubernetesAPIHandler() *KubernetesAPIHandler {
	return &KubernetesAPIHandler{}
}

func (h *KubernetesAPIHandler) RegisterRoutes(group *gin.RouterGroup) {
	group.Any("/kubernetes/*path", h.Proxy)
}

func (h *KubernetesAPIHandler) Proxy(c *gin.Context) {
	defer suppressCanceledProxyAbort(c.Request)

	clientSet := c.MustGet("cluster").(*cluster.ClientSet)
	if clientSet.K8sClient == nil || clientSet.K8sClient.HTTPClient == nil || clientSet.K8sClient.Configuration == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Kubernetes client is unavailable"})
		return
	}
	upstreamPath := c.Param("path")
	if err := validateKubernetesAPIPath(upstreamPath); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	target, err := url.Parse(clientSet.K8sClient.Configuration.Host)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "Kubernetes API server URL is invalid"})
		return
	}

	reverseProxy := httputil.NewSingleHostReverseProxy(target)
	reverseProxy.Transport = clientSet.K8sClient.HTTPClient.Transport
	reverseProxy.FlushInterval = -1
	reverseProxy.ErrorHandler = func(writer http.ResponseWriter, request *http.Request, proxyError error) {
		if errors.Is(proxyError, context.Canceled) || errors.Is(request.Context().Err(), context.Canceled) {
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusBadGateway)
		_, _ = writer.Write([]byte(`{"error":"Kubernetes API request failed"}`))
	}
	baseDirector := reverseProxy.Director
	audit := newKubernetesMutationAudit(c, clientSet, target, upstreamPath)
	if audit != nil {
		reverseProxy.ModifyResponse = audit.record
	}
	reverseProxy.Director = func(request *http.Request) {
		baseDirector(request)
		request.URL.Path = joinURLPath(target.Path, upstreamPath)
		request.URL.RawPath = ""
		request.Host = target.Host
		stripBrowserCredentials(request.Header)
	}
	reverseProxy.ServeHTTP(c.Writer, c.Request)
}

func suppressCanceledProxyAbort(request *http.Request) {
	recovered := recover()
	if recovered == nil {
		return
	}
	proxyError, isError := recovered.(error)
	if isError && errors.Is(proxyError, http.ErrAbortHandler) && errors.Is(request.Context().Err(), context.Canceled) {
		return
	}
	panic(recovered)
}

func validateKubernetesAPIPath(value string) error {
	if value == "" || value[0] != '/' {
		return errors.New("kubernetes API path must be absolute")
	}
	decoded, err := url.PathUnescape(value)
	if err != nil {
		return errors.New("kubernetes API path is invalid")
	}
	for _, segment := range strings.Split(decoded, "/") {
		if segment == ".." {
			return errors.New("kubernetes API path must not contain parent traversal")
		}
	}
	return nil
}

func joinURLPath(base, suffix string) string {
	return strings.TrimRight(base, "/") + "/" + strings.TrimLeft(suffix, "/")
}

func stripBrowserCredentials(header http.Header) {
	header.Del("Authorization")
	header.Del("Cookie")
	header.Del("Proxy-Authorization")
}
