package main

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gin-contrib/gzip"
	"github.com/gin-gonic/gin"
	"github.com/zxh326/kite/internal"
	"github.com/zxh326/kite/pkg/auth"
	"github.com/zxh326/kite/pkg/cluster"
	"github.com/zxh326/kite/pkg/common"
	"github.com/zxh326/kite/pkg/middleware"
	"github.com/zxh326/kite/pkg/model"
	"github.com/zxh326/kite/pkg/resourceapi"
	"github.com/zxh326/kite/pkg/scheduler"
	"github.com/zxh326/kite/pkg/templates"
	"k8s.io/klog/v2"
	controllerlog "sigs.k8s.io/controller-runtime/pkg/log"
)

type application struct {
	clusters    *cluster.ClusterManager
	auth        *auth.AuthHandler
	resourceAPI *resourceapi.Server
	ready       atomic.Bool
}

func initializeApp(ctx context.Context) (*application, error) {
	common.LoadEnvs()
	controllerlog.SetLogger(klog.NewKlogr())
	if err := validateOIDCConfiguration(); err != nil {
		return nil, err
	}
	if err := validateResourceServerConfiguration(); err != nil {
		return nil, err
	}
	if err := validateAnalyticsConfiguration(); err != nil {
		return nil, err
	}
	if common.KiteEncryptKey == "kite-default-encryption-key-change-in-production" {
		return nil, errors.New("KITE_ENCRYPT_KEY must be set because OIDC tokens are stored server-side")
	}
	if common.JwtSecret == common.DefaultJWTSecret {
		return nil, errors.New("JWT_SECRET must be set because it protects cluster tunnel enrollment grants")
	}
	if klog.V(1).Enabled() {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}

	model.InitDB()
	if _, err := model.GetGeneralSetting(); err != nil {
		return nil, errors.New("load general setting: " + err.Error())
	}
	if err := validateAnalyticsConfiguration(); err != nil {
		return nil, err
	}

	if err := templates.InitTemplates(); err != nil {
		return nil, errors.New("initialize resource templates: " + err.Error())
	}
	if err := internal.LoadConfigFromFile(common.ConfigFilePath); err != nil {
		return nil, err
	}

	cm, err := cluster.NewClusterManager()
	if err != nil {
		return nil, err
	}
	if err := internal.StartConfigWatcher(ctx, common.ConfigFilePath, func(sections internal.AppliedSections) {
		if sections["clusters"] {
			cm.InvalidateCatalogRuntimes()
		}
	}); err != nil {
		klog.Warningf("Failed to watch config file: %v", err)
	}
	authHandler := auth.NewAuthHandler()
	var resourceAPI *resourceapi.Server
	if common.ResourceServerEnabled() {
		resourceAPI, err = resourceapi.New(ctx, common.ResourceServerURL, common.ResourceServerIssuer, common.ResourceServerClients, common.ResourceServerJWTAlgs, cm, model.DB)
		if err != nil {
			return nil, err
		}
	}
	scheduler.Start(ctx, cm, authHandler)
	app := &application{clusters: cm, auth: authHandler, resourceAPI: resourceAPI}
	app.ready.Store(true)
	return app, nil
}

func validateResourceServerConfiguration() error {
	if !common.ResourceServerEnabled() {
		return nil
	}
	if common.ResourceServerIssuer == "" {
		return errors.New("RESOURCE_SERVER_ISSUER is required when RESOURCE_SERVER_URL is set")
	}
	if len(common.ResourceServerClients) == 0 {
		return errors.New("RESOURCE_SERVER_AUTHORIZED_CLIENT_IDS is required when RESOURCE_SERVER_URL is set")
	}
	if len(common.ResourceServerJWTAlgs) == 0 {
		return errors.New("RESOURCE_SERVER_JWT_ALGORITHMS must contain at least one signing algorithm")
	}
	if err := validateExternalURL("RESOURCE_SERVER_URL", common.ResourceServerURL, true); err != nil {
		return err
	}
	if err := validateExternalURL("RESOURCE_SERVER_ISSUER", common.ResourceServerIssuer, true); err != nil {
		return err
	}
	return nil
}

func validateAnalyticsConfiguration() error {
	scriptConfigured := common.AnalyticsScriptURL != ""
	websiteConfigured := common.AnalyticsWebsiteID != ""
	if scriptConfigured != websiteConfigured {
		return errors.New("ANALYTICS_SCRIPT_URL and ANALYTICS_WEBSITE_ID must be configured together")
	}
	if common.EnableAnalytics && !common.AnalyticsConfigured() {
		return errors.New("ANALYTICS_SCRIPT_URL and ANALYTICS_WEBSITE_ID are required when analytics is enabled")
	}
	if scriptConfigured {
		if err := validateExternalURL("ANALYTICS_SCRIPT_URL", common.AnalyticsScriptURL, true); err != nil {
			return err
		}
	}
	return nil
}

func validateOIDCConfiguration() error {
	if common.OIDCIssuer == "" || common.OIDCClientID == "" || common.OIDCClientSecret == "" {
		return errors.New("OIDC_ISSUER, OIDC_CLIENT_ID, and OIDC_CLIENT_SECRET are required")
	}
	if len(common.PlatformAdminGroups) == 0 && len(common.PlatformAdminSubjects) == 0 {
		return errors.New("PLATFORM_ADMIN_GROUPS or PLATFORM_ADMIN_SUBJECTS must grant cluster catalog administration")
	}
	if common.OIDCUsernameClaim == "" || common.OIDCGroupsClaim == "" {
		return errors.New("OIDC_USERNAME_CLAIM and OIDC_GROUPS_CLAIM are required")
	}
	if !containsString(common.OIDCScopes, "openid") {
		return errors.New("OIDC_SCOPES must include openid")
	}
	if !containsString(common.OIDCScopes, "offline_access") {
		return errors.New("OIDC_SCOPES must include offline_access for user-authorized scheduled operations")
	}
	if err := validateExternalURL("HOST", common.Host, false); err != nil {
		return err
	}
	if err := validateExternalURL("OIDC_ISSUER", common.OIDCIssuer, true); err != nil {
		return err
	}
	if common.ReleaseAPIURL != "" {
		if err := validateExternalURL("RELEASE_API_URL", common.ReleaseAPIURL, true); err != nil {
			return err
		}
	}
	return nil
}

func validateExternalURL(name, value string, allowPath bool) error {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return errors.New(name + " must be an absolute URL without credentials, query, or fragment")
	}
	if !allowPath && strings.Trim(parsed.Path, "/") != "" {
		return errors.New(name + " must be an origin without a path; use KITE_BASE for a path prefix")
	}
	if parsed.Scheme != "https" && (parsed.Scheme != "http" || !isLoopbackHostname(parsed.Hostname())) {
		return errors.New(name + " must use HTTPS except for a loopback development address")
	}
	return nil
}

func isLoopbackHostname(hostname string) bool {
	if strings.EqualFold(hostname, "localhost") {
		return true
	}
	address := net.ParseIP(hostname)
	return address != nil && address.IsLoopback()
}

func (app *application) readiness(c *gin.Context) {
	if !app.ready.Load() || model.DB == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "not ready"})
		return
	}
	sqlDB, err := model.DB.DB()
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "not ready"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), time.Second)
	defer cancel()
	if err := sqlDB.PingContext(ctx); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "not ready"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ready"})
}

func containsString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func buildEngine(app *application) *gin.Engine {
	r := gin.New()
	middleware.ConfigureRawPathRouting(r)
	configureTrustedProxies(r)
	r.Use(middleware.Metrics())
	if !common.DisableGZIP {
		klog.Info("GZIP compression is enabled")
		r.Use(gzip.Gzip(gzip.DefaultCompression, gzip.WithExcludedPaths([]string{"/metrics"})))
	}
	r.Use(gin.Recovery())
	r.Use(middleware.Logger())
	r.Use(middleware.DevCORS(common.CORSAllowedOrigins))
	if app.resourceAPI != nil {
		app.resourceAPI.Register(r)
	}

	base := r.Group(common.Base)
	setupAPIRouter(base, app)
	setupStatic(r)

	return r
}

func configureTrustedProxies(r *gin.Engine) {
	var trustedProxies []string
	if len(common.TrustedProxies) > 0 {
		trustedProxies = common.TrustedProxies
	}
	if err := r.SetTrustedProxies(trustedProxies); err != nil {
		klog.Fatalf("Failed to configure trusted proxies: %v", err)
	}
}
