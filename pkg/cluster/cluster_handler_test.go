package cluster

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/zxh326/kite/pkg/common"
	"github.com/zxh326/kite/pkg/model"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestCredentialFreeClusterConfigurationLifecycle(t *testing.T) {
	setupClusterHandlerTestDB(t)
	router := newClusterHandlerTestRouter()
	primary := createPrimaryCluster(t, router)
	if !primary.IsDefault || !primary.Enable || primary.APIServerURL != "https://k8s.example.com" {
		t.Fatalf("created cluster = %#v", primary)
	}

	updateBody := `{"name":"renamed","description":"updated","apiServerUrl":"https://new-k8s.example.com","prometheusURL":"http://prometheus.monitoring.svc:9090","isDefault":true,"enabled":true}`
	update := performClusterRequest(router, http.MethodPut, fmt.Sprintf("/clusters/%d", primary.ID), updateBody)
	if update.Code != http.StatusOK {
		t.Fatalf("update status = %d, want %d: %s", update.Code, http.StatusOK, update.Body.String())
	}
	updated, err := model.GetClusterByID(primary.ID)
	if err != nil {
		t.Fatalf("loading updated cluster: %v", err)
	}
	if updated.Name != "renamed" || updated.Description != "updated" || updated.PrometheusURL != "http://prometheus.monitoring.svc:9090" {
		t.Fatalf("updated cluster = %#v", updated)
	}
	if updated.APIServerURL != "https://new-k8s.example.com" || updated.CABundle != "" {
		t.Fatalf("updated cluster lost transport metadata: %#v", updated)
	}
	invalidURL := performClusterRequest(router, http.MethodPut, fmt.Sprintf("/clusters/%d", primary.ID),
		`{"name":"renamed","apiServerUrl":"http://user:password@new-k8s.example.com?token=secret","enabled":true}`)
	if invalidURL.Code != http.StatusBadRequest {
		t.Fatalf("invalid URL update status = %d, want %d: %s", invalidURL.Code, http.StatusBadRequest, invalidURL.Body.String())
	}
	invalidCA := performClusterRequest(router, http.MethodPut, fmt.Sprintf("/clusters/%d", primary.ID),
		`{"name":"renamed","apiServerUrl":"https://new-k8s.example.com","caBundle":"not-a-certificate","enabled":true}`)
	if invalidCA.Code != http.StatusBadRequest {
		t.Fatalf("invalid CA update status = %d, want %d: %s", invalidCA.Code, http.StatusBadRequest, invalidCA.Body.String())
	}

	list := performClusterRequest(router, http.MethodGet, "/clusters", "")
	if list.Code != http.StatusOK {
		t.Fatalf("list status = %d, want %d", list.Code, http.StatusOK)
	}
	var listed []map[string]any
	if err := json.Unmarshal(list.Body.Bytes(), &listed); err != nil {
		t.Fatalf("decoding list response: %v", err)
	}
	if len(listed) != 1 || listed[0]["config"] != nil || listed[0]["apiServerUrl"] != "https://new-k8s.example.com" {
		t.Fatalf("listed clusters = %#v", listed)
	}
	if strings.Contains(list.Body.String(), "kubeconfig") {
		t.Fatal("cluster list exposed kubeconfig")
	}
}

func TestCredentialFreeClusterDefaultAndDeletionConstraints(t *testing.T) {
	setupClusterHandlerTestDB(t)
	router := newClusterHandlerTestRouter()
	primary := createPrimaryCluster(t, router)

	deleteDefault := performClusterRequest(router, http.MethodDelete, fmt.Sprintf("/clusters/%d", primary.ID), "")
	if deleteDefault.Code != http.StatusBadRequest {
		t.Fatalf("default delete status = %d, want %d", deleteDefault.Code, http.StatusBadRequest)
	}

	secondary := &model.Cluster{Name: "secondary", APIServerURL: "https://secondary.example.com", Enable: true}
	if err := model.AddCluster(secondary); err != nil {
		t.Fatalf("creating secondary cluster: %v", err)
	}
	duplicateName := performClusterRequest(router, http.MethodPut, fmt.Sprintf("/clusters/%d", secondary.ID),
		`{"name":"primary","apiServerUrl":"https://secondary.example.com","enabled":true}`)
	if duplicateName.Code != http.StatusConflict {
		t.Fatalf("duplicate update status = %d, want %d: %s", duplicateName.Code, http.StatusConflict, duplicateName.Body.String())
	}

	if err := model.UpdateCluster(secondary, map[string]interface{}{"name": "primary", "is_default": true}); err == nil {
		t.Fatal("duplicate default update unexpectedly succeeded")
	}
	reloadedPrimary, err := model.GetClusterByID(primary.ID)
	if err != nil {
		t.Fatal(err)
	}
	reloadedSecondary, err := model.GetClusterByID(secondary.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reloadedPrimary.IsDefault || reloadedSecondary.IsDefault {
		t.Fatalf("failed default switch was not rolled back: primary=%t secondary=%t", reloadedPrimary.IsDefault, reloadedSecondary.IsDefault)
	}
	deleteSecondary := performClusterRequest(router, http.MethodDelete, fmt.Sprintf("/clusters/%d", secondary.ID), "")
	if deleteSecondary.Code != http.StatusOK {
		t.Fatalf("secondary delete status = %d, want %d: %s", deleteSecondary.Code, http.StatusOK, deleteSecondary.Body.String())
	}
	if _, err := model.GetClusterByID(secondary.ID); err == nil {
		t.Fatal("deleted secondary cluster still exists")
	}
	recreated := &model.Cluster{Name: "secondary", APIServerURL: "https://replacement.example.com", Enable: true}
	if err := model.AddCluster(recreated); err != nil {
		t.Fatalf("recreating deleted cluster name: %v", err)
	}
}

func newClusterHandlerTestRouter() *gin.Engine {
	manager := &ClusterManager{}
	router := gin.New()
	router.POST("/clusters", manager.CreateCluster)
	router.GET("/clusters", manager.GetClusterList)
	router.PUT("/clusters/:id", manager.UpdateCluster)
	router.DELETE("/clusters/:id", manager.DeleteCluster)
	return router
}

func createPrimaryCluster(t *testing.T, router *gin.Engine) *model.Cluster {
	t.Helper()
	create := performClusterRequest(router, http.MethodPost, "/clusters", `{"name":"primary","description":"main","apiServerUrl":"https://k8s.example.com","isDefault":true}`)
	if create.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want %d: %s", create.Code, http.StatusCreated, create.Body.String())
	}
	primary, err := model.GetClusterByName("primary")
	if err != nil {
		t.Fatalf("loading created cluster: %v", err)
	}
	return primary
}

func TestCreateClusterAcceptsFrontendEnabledField(t *testing.T) {
	setupClusterHandlerTestDB(t)
	router := newClusterHandlerTestRouter()
	response := performClusterRequest(router, http.MethodPost, "/clusters", `{"name":"disabled","apiServerUrl":"https://k8s.example.com","enabled":false}`)
	if response.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want %d: %s", response.Code, http.StatusCreated, response.Body.String())
	}
	created, err := model.GetClusterByName("disabled")
	if err != nil {
		t.Fatal(err)
	}
	if created.Enable {
		t.Fatal("explicit enabled=false was ignored")
	}
}

func TestGetClusterListUsesCredentialFreeCatalog(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupClusterHandlerTestDB(t)
	for _, cluster := range []*model.Cluster{
		{Name: "first", APIServerURL: "https://first.example.com", Enable: true},
		{Name: "second", APIServerURL: "https://second.example.com", Enable: true, IsDefault: true},
	} {
		if err := model.AddCluster(cluster); err != nil {
			t.Fatal(err)
		}
	}
	manager := &ClusterManager{}
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/clusters", nil)

	manager.GetClusterList(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	var result []map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if len(result) != 2 || result[0]["apiServerUrl"] != "https://first.example.com" || result[1]["isDefault"] != true {
		t.Fatalf("clusters = %#v", result)
	}
	if result[0]["config"] != nil || result[1]["config"] != nil {
		t.Fatalf("cluster catalog exposed credentials: %#v", result)
	}
}

func TestClusterMutationsHonorManagedConfiguration(t *testing.T) {
	setupClusterHandlerTestDB(t)
	common.SetManagedSections(map[string]bool{"clusters": true})
	manager := &ClusterManager{}
	router := gin.New()
	router.POST("/clusters", manager.CreateCluster)

	response := performClusterRequest(router, http.MethodPost, "/clusters", `{"name":"blocked","apiServerUrl":"https://blocked.example.test"}`)
	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusForbidden)
	}
	var count int64
	if err := model.DB.Model(&model.Cluster{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("managed mutation persisted clusters: count=%d err=%v", count, err)
	}
}

func setupClusterHandlerTestDB(t *testing.T) {
	t.Helper()
	originalDB := model.DB
	originalEncryptKey := common.KiteEncryptKey
	originalHost := common.Host
	originalBase := common.Base
	originalManagedSections := make(map[string]bool, len(common.ManagedSections))
	for section, managed := range common.ManagedSections {
		originalManagedSections[section] = managed
	}
	common.KiteEncryptKey = "cluster-handler-test-key"
	common.Host = "https://kite.example.test"
	common.Base = ""
	common.SetManagedSections(nil)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("opening test database: %v", err)
	}
	if err := db.AutoMigrate(&model.Cluster{}, &model.GeneralSetting{}); err != nil {
		t.Fatalf("migrating test database: %v", err)
	}
	model.DB = db
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
		model.DB = originalDB
		common.KiteEncryptKey = originalEncryptKey
		common.Host = originalHost
		common.Base = originalBase
		common.SetManagedSections(originalManagedSections)
	})
}

func performClusterRequest(router *gin.Engine, method string, path string, body string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	router.ServeHTTP(recorder, request)
	return recorder
}
