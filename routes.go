package main

import (
	"net/http"

	"github.com/gin-gonic/gin"
	promclient "github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/zxh326/kite/pkg/audit"
	"github.com/zxh326/kite/pkg/auth"
	"github.com/zxh326/kite/pkg/cluster"
	"github.com/zxh326/kite/pkg/helm"
	"github.com/zxh326/kite/pkg/images"
	"github.com/zxh326/kite/pkg/metrics"
	"github.com/zxh326/kite/pkg/middleware"
	"github.com/zxh326/kite/pkg/proxy"
	"github.com/zxh326/kite/pkg/resources"
	"github.com/zxh326/kite/pkg/search"
	"github.com/zxh326/kite/pkg/settings"
	"github.com/zxh326/kite/pkg/system"
	"github.com/zxh326/kite/pkg/templates"
	"github.com/zxh326/kite/pkg/terminal"
	"github.com/zxh326/kite/pkg/users"
	"github.com/zxh326/kite/pkg/version"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

func setupAPIRouter(r *gin.RouterGroup, app *application) {
	cm := app.clusters
	authHandler := app.auth
	helmChartsHandler := helm.NewHelmChartHandler()

	registerBaseRoutes(r, app)
	r.GET("/api/v1/bootstrap", authHandler.Bootstrap)
	registerAuthRoutes(r, authHandler)
	registerUserRoutes(r, authHandler)
	registerAdminRoutes(r, authHandler, cm, helmChartsHandler)
	registerProtectedRoutes(r, authHandler, cm, helmChartsHandler)
}

func registerBaseRoutes(r *gin.RouterGroup, app *application) {
	r.GET("/metrics", gin.WrapH(promhttp.HandlerFor(promclient.Gatherers{
		promclient.DefaultGatherer,
		ctrlmetrics.Registry,
	}, promhttp.HandlerOpts{})))
	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	r.GET("/readyz", app.readiness)
	r.GET("/api/v1/version", version.GetVersion)
}

func registerAuthRoutes(r *gin.RouterGroup, authHandler *auth.AuthHandler) {
	authGroup := r.Group("/api/auth")
	authGroup.GET("/login", authHandler.Login)
	authGroup.GET("/callback", authHandler.Callback)
	authGroup.POST("/logout", authHandler.Logout)
	authGroup.POST("/refresh", authHandler.RefreshToken)
	authGroup.GET("/user", authHandler.RequireAuth(), authHandler.GetUser)
}

func registerUserRoutes(r *gin.RouterGroup, authHandler *auth.AuthHandler) {
	userGroup := r.Group("/api/users")
	userGroup.POST("/sidebar_preference", authHandler.RequireAuth(), users.UpdateSidebarPreference)
}

func registerAdminRoutes(r *gin.RouterGroup, authHandler *auth.AuthHandler, cm *cluster.ClusterManager, helmChartsHandler *helm.HelmChartHandler) {
	adminAPI := r.Group("/api/v1/admin")
	adminAPI.Use(authHandler.RequireAuth(), authHandler.RequireAdmin())

	adminAPI.GET("/audit-logs", audit.ListAuditLogs)
	adminAPI.GET("/general-setting/", settings.HandleGetGeneralSetting)
	adminAPI.PUT("/general-setting/", settings.HandleUpdateGeneralSetting)

	clusterAPI := adminAPI.Group("/clusters")
	clusterAPI.GET("/", cm.GetClusterList)
	clusterAPI.POST("/", cm.CreateCluster)
	clusterAPI.PUT("/:id", cm.UpdateCluster)
	clusterAPI.DELETE("/:id", cm.DeleteCluster)

	adminAPI.POST("/sidebar_preference/global", users.UpdateGlobalSidebarPreference)
	adminAPI.DELETE("/sidebar_preference/global", users.ClearGlobalSidebarPreference)

	templateAPI := adminAPI.Group("/templates")
	templateAPI.POST("/", templates.CreateTemplate)
	templateAPI.PUT("/:id", templates.UpdateTemplate)
	templateAPI.DELETE("/:id", templates.DeleteTemplate)

	helmChartsHandler.RegisterAdminRoutes(adminAPI)
}

func registerProtectedRoutes(r *gin.RouterGroup, authHandler *auth.AuthHandler, cm *cluster.ClusterManager, helmChartsHandler *helm.HelmChartHandler) {
	metricsHandler := metrics.NewHandler()
	searchHandler := search.NewSearchHandler(resources.SearchFuncs())
	api := r.Group("/api/v1")
	api.GET("/clusters", authHandler.RequireAuth(), cm.GetClusters)
	defaultAPI := api.Group("")
	defaultAPI.Use(authHandler.RequireAuth(), middleware.ClusterMiddleware(cm))
	registerClusterProtectedRoutes(defaultAPI, helmChartsHandler, metricsHandler, searchHandler)

	clusterAPI := api.Group("/_clusters/:cluster")
	clusterAPI.Use(authHandler.RequireAuth(), middleware.ClusterMiddleware(cm))
	registerClusterProtectedRoutes(clusterAPI, helmChartsHandler, metricsHandler, searchHandler)
}

func registerClusterProtectedRoutes(
	api *gin.RouterGroup,
	helmChartsHandler *helm.HelmChartHandler,
	metricsHandler *metrics.Handler,
	searchHandler *search.SearchHandler,
) {
	api.GET("/overview", system.GetOverview)

	api.GET("/prometheus/resource-usage-history", metricsHandler.GetResourceUsageHistory)
	api.GET("/prometheus/pods/:namespace/:podName/metrics", metricsHandler.GetPodMetrics)

	logsHandler := resources.NewLogsHandler()
	api.GET("/logs/:namespace/:podName/ws", logsHandler.HandleLogsWebSocket)

	terminalHandler := terminal.NewTerminalHandler()
	api.GET("/terminal/:namespace/:podName/ws", terminalHandler.HandleTerminalWebSocket)
	nodeTerminalHandler := terminal.NewNodeTerminalHandler()
	api.GET("/node-terminal/:nodeName/ws", nodeTerminalHandler.HandleNodeTerminalWebSocket)
	kubectlTerminalHandler := terminal.NewKubectlTerminalHandler()
	api.GET("/kubectl-terminal/ws", kubectlTerminalHandler.HandleKubectlTerminalWebSocket)

	api.GET("/search", searchHandler.GlobalSearch)

	resourceApplyHandler := resources.NewResourceApplyHandler()
	api.POST("/resources/apply", resourceApplyHandler.ApplyResource)

	api.GET("/image/tags", images.GetImageTags)
	api.GET("/templates", templates.ListTemplates)

	helmChartsHandler.RegisterRoutes(api)

	proxy.NewKubernetesAPIHandler().RegisterRoutes(api)

	resources.RegisterRoutes(api)
}
