package settings

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/realmroot/lightkite/pkg/common"
	"github.com/realmroot/lightkite/pkg/model"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestGeneralSettingHandlersReadAndUpdateRuntimeSettings(t *testing.T) {
	setupGeneralSettingTestDB(t)
	common.AnalyticsScriptURL = "https://analytics.example.test/script.js"
	common.AnalyticsWebsiteID = "lightkite-test"
	router := generalSettingTestRouter()

	getResponse := performGeneralSettingRequest(t, router, http.MethodGet, "")
	if getResponse.Code != http.StatusOK {
		t.Fatalf("GET status = %d, want %d: %s", getResponse.Code, http.StatusOK, getResponse.Body.String())
	}
	getBody := decodeGeneralSettingResponse(t, getResponse)
	if getBody["enableAnalytics"] != false {
		t.Fatalf("enableAnalytics = %#v, want false", getBody["enableAnalytics"])
	}
	if getBody["analyticsConfigured"] != true {
		t.Fatalf("analyticsConfigured = %#v, want true", getBody["analyticsConfigured"])
	}

	updateBody := `{"enableAnalytics":true,"enableVersionCheck":false}`
	updateResponse := performGeneralSettingRequest(t, router, http.MethodPut, updateBody)
	if updateResponse.Code != http.StatusOK {
		t.Fatalf("PUT status = %d, want %d: %s", updateResponse.Code, http.StatusOK, updateResponse.Body.String())
	}
	responseBody := decodeGeneralSettingResponse(t, updateResponse)
	if responseBody["enableAnalytics"] != true || responseBody["enableVersionCheck"] != false {
		t.Fatalf("PUT runtime fields = %#v", responseBody)
	}
}

func TestGeneralSettingHandlerRejectsAnalyticsWithoutOperatorConfiguration(t *testing.T) {
	setupGeneralSettingTestDB(t)
	common.AnalyticsScriptURL = ""
	common.AnalyticsWebsiteID = ""

	response := performGeneralSettingRequest(t, generalSettingTestRouter(), http.MethodPut, `{"enableAnalytics":true}`)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("PUT status = %d, want %d: %s", response.Code, http.StatusBadRequest, response.Body.String())
	}
}

func setupGeneralSettingTestDB(t *testing.T) {
	t.Helper()
	originalDB := model.DB
	originalEncryptKey := common.LightkiteEncryptKey
	originalAnalytics := common.EnableAnalytics
	originalAnalyticsScriptURL := common.AnalyticsScriptURL
	originalAnalyticsWebsiteID := common.AnalyticsWebsiteID
	originalVersionCheck := common.EnableVersionCheck

	common.LightkiteEncryptKey = "settings-handler-test-key"
	common.EnableAnalytics = false
	common.AnalyticsScriptURL = ""
	common.AnalyticsWebsiteID = ""
	common.EnableVersionCheck = true

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("opening test database: %v", err)
	}
	if err := db.AutoMigrate(&model.GeneralSetting{}); err != nil {
		t.Fatalf("migrating test database: %v", err)
	}
	model.DB = db
	setting := model.GeneralSetting{
		Model:              model.Model{ID: 1},
		KubectlEnabled:     true,
		KubectlImage:       model.DefaultGeneralKubectlImage,
		NodeTerminalImage:  model.DefaultGeneralNodeTerminalImage,
		EnableVersionCheck: true,
	}
	if err := db.Create(&setting).Error; err != nil {
		t.Fatalf("creating general setting: %v", err)
	}

	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
		model.DB = originalDB
		common.LightkiteEncryptKey = originalEncryptKey
		common.EnableAnalytics = originalAnalytics
		common.AnalyticsScriptURL = originalAnalyticsScriptURL
		common.AnalyticsWebsiteID = originalAnalyticsWebsiteID
		common.EnableVersionCheck = originalVersionCheck
	})
}

func generalSettingTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/settings", HandleGetGeneralSetting)
	router.PUT("/settings", HandleUpdateGeneralSetting)
	return router
}

func performGeneralSettingRequest(t *testing.T, router *gin.Engine, method string, body string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, "/settings", strings.NewReader(body))
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	router.ServeHTTP(recorder, request)
	return recorder
}

func decodeGeneralSettingResponse(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	return body
}
