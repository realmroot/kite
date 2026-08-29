package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/realmroot/lightkite/pkg/common"
	"github.com/realmroot/lightkite/pkg/model"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func withValidOIDCConfiguration(t *testing.T) {
	t.Helper()
	previousIssuer := common.OIDCIssuer
	previousClientID := common.OIDCClientID
	previousClientSecret := common.OIDCClientSecret
	previousHost := common.Host
	previousScopes := append([]string(nil), common.OIDCScopes...)
	previousUsernameClaim := common.OIDCUsernameClaim
	previousGroupsClaim := common.OIDCGroupsClaim
	previousReleaseAPIURL := common.ReleaseAPIURL
	previousAdminGroups := append([]string(nil), common.PlatformAdminGroups...)
	previousAdminSubjects := append([]string(nil), common.PlatformAdminSubjects...)
	t.Cleanup(func() {
		common.OIDCIssuer = previousIssuer
		common.OIDCClientID = previousClientID
		common.OIDCClientSecret = previousClientSecret
		common.Host = previousHost
		common.OIDCScopes = previousScopes
		common.OIDCUsernameClaim = previousUsernameClaim
		common.OIDCGroupsClaim = previousGroupsClaim
		common.ReleaseAPIURL = previousReleaseAPIURL
		common.PlatformAdminGroups = previousAdminGroups
		common.PlatformAdminSubjects = previousAdminSubjects
	})
	common.OIDCIssuer = "https://identity.example.com/tenant"
	common.OIDCClientID = "lightkite"
	common.OIDCClientSecret = "secret"
	common.Host = "https://lightkite.example.com"
	common.OIDCScopes = []string{"openid", "profile", "offline_access"}
	common.OIDCUsernameClaim = "email"
	common.OIDCGroupsClaim = "groups"
	common.ReleaseAPIURL = ""
	common.PlatformAdminGroups = []string{"admins"}
	common.PlatformAdminSubjects = nil
}

func TestValidateOIDCConfigurationAcceptsSecureStandardConfiguration(t *testing.T) {
	withValidOIDCConfiguration(t)
	if err := validateOIDCConfiguration(); err != nil {
		t.Fatal(err)
	}
}

func TestValidateOIDCConfigurationRejectsUnsafeOrIncompleteURLs(t *testing.T) {
	tests := []struct {
		name   string
		mutate func()
	}{
		{name: "missing public host", mutate: func() { common.Host = "" }},
		{name: "host path", mutate: func() { common.Host = "https://lightkite.example.com/app" }},
		{name: "host credentials", mutate: func() { common.Host = "https://user@lightkite.example.com" }},
		{name: "insecure public host", mutate: func() { common.Host = "http://lightkite.example.com" }},
		{name: "insecure issuer", mutate: func() { common.OIDCIssuer = "http://identity.example.com" }},
		{name: "issuer query", mutate: func() { common.OIDCIssuer = "https://identity.example.com?tenant=one" }},
		{name: "insecure release API", mutate: func() { common.ReleaseAPIURL = "http://code.example.com/releases/latest" }},
		{name: "missing offline access", mutate: func() { common.OIDCScopes = []string{"openid"} }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			withValidOIDCConfiguration(t)
			test.mutate()
			if err := validateOIDCConfiguration(); err == nil {
				t.Fatal("invalid configuration was accepted")
			}
		})
	}
}

func TestValidateOIDCConfigurationAllowsLoopbackHTTPForDevelopment(t *testing.T) {
	withValidOIDCConfiguration(t)
	common.Host = "http://127.0.0.1:8080"
	common.OIDCIssuer = "http://localhost:5556/dex"
	if err := validateOIDCConfiguration(); err != nil {
		t.Fatal(err)
	}
}

func TestValidateAnalyticsConfiguration(t *testing.T) {
	previousEnabled := common.EnableAnalytics
	previousScriptURL := common.AnalyticsScriptURL
	previousWebsiteID := common.AnalyticsWebsiteID
	t.Cleanup(func() {
		common.EnableAnalytics = previousEnabled
		common.AnalyticsScriptURL = previousScriptURL
		common.AnalyticsWebsiteID = previousWebsiteID
	})

	tests := []struct {
		name      string
		enabled   bool
		scriptURL string
		websiteID string
		wantError bool
	}{
		{name: "disabled and unconfigured"},
		{name: "configured but disabled", scriptURL: "https://analytics.example.com/script.js", websiteID: "site"},
		{name: "enabled and configured", enabled: true, scriptURL: "https://analytics.example.com/script.js", websiteID: "site"},
		{name: "enabled but unconfigured", enabled: true, wantError: true},
		{name: "partial configuration", scriptURL: "https://analytics.example.com/script.js", wantError: true},
		{name: "insecure public script", scriptURL: "http://analytics.example.com/script.js", websiteID: "site", wantError: true},
		{name: "script credentials", scriptURL: "https://user@analytics.example.com/script.js", websiteID: "site", wantError: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			common.EnableAnalytics = test.enabled
			common.AnalyticsScriptURL = test.scriptURL
			common.AnalyticsWebsiteID = test.websiteID
			err := validateAnalyticsConfiguration()
			if (err != nil) != test.wantError {
				t.Fatalf("validateAnalyticsConfiguration() error = %v, wantError %v", err, test.wantError)
			}
		})
	}
}

func TestReadinessReflectsLifecycleAndDatabaseAvailability(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	previousDB := model.DB
	model.DB = db
	t.Cleanup(func() { model.DB = previousDB })
	app := &application{}
	app.ready.Store(true)

	router := gin.New()
	router.GET("/readyz", app.readiness)
	request := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("ready status = %d, body = %s", response.Code, response.Body.String())
	}

	app.ready.Store(false)
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("draining status = %d, body = %s", response.Code, response.Body.String())
	}
}
