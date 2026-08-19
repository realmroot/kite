package main

import (
	"context"
	"errors"

	"github.com/gin-contrib/gzip"
	"github.com/gin-gonic/gin"
	"github.com/zxh326/kite/internal"
	"github.com/zxh326/kite/pkg/cluster"
	"github.com/zxh326/kite/pkg/common"
	"github.com/zxh326/kite/pkg/middleware"
	"github.com/zxh326/kite/pkg/model"
	"github.com/zxh326/kite/pkg/templates"
	"k8s.io/klog/v2"
)

func initializeApp(ctx context.Context) (*cluster.ClusterManager, error) {
	common.LoadEnvs()
	if err := validateOIDCConfiguration(); err != nil {
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
		klog.Warningf("Failed to load general setting: %v", err)
	}

	templates.InitTemplates()
	internal.LoadConfigFromFile(common.ConfigFilePath)

	cm, err := cluster.NewClusterManager()
	if err != nil {
		return nil, err
	}
	if err := internal.StartConfigWatcher(ctx, common.ConfigFilePath); err != nil {
		klog.Warningf("Failed to watch config file: %v", err)
	}
	return cm, nil
}

func validateOIDCConfiguration() error {
	if common.OIDCIssuer == "" || common.OIDCClientID == "" || common.OIDCClientSecret == "" {
		return errors.New("OIDC_ISSUER, OIDC_CLIENT_ID, and OIDC_CLIENT_SECRET are required")
	}
	if len(common.PlatformAdminGroups) == 0 {
		return errors.New("PLATFORM_ADMIN_GROUPS must name at least one group that can manage the cluster catalog")
	}
	if common.OIDCUsernameClaim == "" || common.OIDCGroupsClaim == "" {
		return errors.New("OIDC_USERNAME_CLAIM and OIDC_GROUPS_CLAIM are required")
	}
	if !containsString(common.OIDCScopes, "openid") {
		return errors.New("OIDC_SCOPES must include openid")
	}
	return nil
}

func containsString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func buildEngine(cm *cluster.ClusterManager) *gin.Engine {
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

	base := r.Group(common.Base)
	setupAPIRouter(base, cm)
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
