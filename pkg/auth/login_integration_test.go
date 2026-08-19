package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/zxh326/kite/pkg/common"
	"github.com/zxh326/kite/pkg/model"
	"github.com/zxh326/kite/pkg/rbac"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestPasswordLoginHonorsMFAAndGlobalDisable(t *testing.T) {
	t.Run("MFA code required", func(t *testing.T) {
		user := setupAuthIntegrationDB(t)
		if err := model.StoreMFASecret(user.ID, "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"); err != nil {
			t.Fatalf("storing MFA secret: %v", err)
		}
		if err := model.EnableUserMFA(user.ID); err != nil {
			t.Fatalf("enabling MFA: %v", err)
		}
		handler := NewAuthHandler()
		router := gin.New()
		router.POST("/login", handler.PasswordLogin)

		response := performAuthRequest(router, http.MethodPost, "/login", `{"username":"alice","password":"correct-password"}`, nil)
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want %d", response.Code, http.StatusUnauthorized)
		}
		var body map[string]string
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
			t.Fatalf("decoding response: %v", err)
		}
		if body["error"] != "mfa_required" {
			t.Fatalf("error = %q, want mfa_required", body["error"])
		}
	})

	t.Run("password login disabled", func(t *testing.T) {
		setupAuthIntegrationDB(t)
		if err := model.DB.Model(&model.GeneralSetting{}).Where("id = ?", 1).Update("password_login_disabled", true).Error; err != nil {
			t.Fatalf("disabling password login: %v", err)
		}
		handler := NewAuthHandler()
		router := gin.New()
		router.POST("/login", handler.PasswordLogin)

		response := performAuthRequest(router, http.MethodPost, "/login", `{"username":"alice","password":"correct-password"}`, nil)
		if response.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want %d", response.Code, http.StatusForbidden)
		}
	})
}

func setupAuthIntegrationDB(t *testing.T) *model.User {
	t.Helper()
	originalDB := model.DB
	originalEncryptKey := common.KiteEncryptKey
	originalJWTSecret := common.JwtSecret
	originalAnalytics := common.EnableAnalytics
	originalVersionCheck := common.EnableVersionCheck
	originalRBACConfig := rbac.RBACConfig
	originalLimiter := credentialLoginAttempts

	common.KiteEncryptKey = "auth-integration-test-key"
	common.JwtSecret = "auth-integration-test-jwt-secret"
	common.EnableAnalytics = false
	common.EnableVersionCheck = true
	credentialLoginAttempts = &credentialLoginAttemptLimiter{attempts: map[string]credentialLoginAttemptState{}}
	rbac.RBACConfig = &common.RolesConfig{
		Roles: []common.Role{{Name: "viewer"}},
		RoleMapping: []common.RoleMapping{{
			Name:  "viewer",
			Users: []string{"alice"},
		}},
	}

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("opening test database: %v", err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.GeneralSetting{}); err != nil {
		t.Fatalf("migrating test database: %v", err)
	}
	model.DB = db
	setting := model.GeneralSetting{
		Model:              model.Model{ID: 1},
		AIProvider:         model.DefaultGeneralAIProvider,
		AIModel:            model.DefaultGeneralAIModel,
		AIMaxTokens:        4096,
		KubectlEnabled:     true,
		KubectlImage:       model.DefaultGeneralKubectlImage,
		NodeTerminalImage:  model.DefaultGeneralNodeTerminalImage,
		EnableVersionCheck: true,
		EnableMFA:          true,
		EnablePasskeyLogin: true,
		JWTSecret:          model.SecretString(common.JwtSecret),
	}
	if err := db.Create(&setting).Error; err != nil {
		t.Fatalf("creating general setting: %v", err)
	}
	user := &model.User{Username: "alice", Password: "correct-password", Provider: model.AuthProviderPassword, Enabled: true}
	if err := model.AddUser(user); err != nil {
		t.Fatalf("creating user: %v", err)
	}
	model.InvalidateUserCache(uint64(user.ID))
	gin.SetMode(gin.TestMode)

	t.Cleanup(func() {
		model.InvalidateUserCache(uint64(user.ID))
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
		model.DB = originalDB
		common.KiteEncryptKey = originalEncryptKey
		common.JwtSecret = originalJWTSecret
		common.EnableAnalytics = originalAnalytics
		common.EnableVersionCheck = originalVersionCheck
		rbac.RBACConfig = originalRBACConfig
		credentialLoginAttempts = originalLimiter
	})
	return user
}

func performAuthRequest(router *gin.Engine, method string, path string, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.RemoteAddr = "192.0.2.25:12345"
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	if cookie != nil {
		request.AddCookie(cookie)
	}
	router.ServeHTTP(recorder, request)
	return recorder
}
