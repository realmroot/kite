package resources

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/realmroot/lightkite/pkg/cluster"
	"github.com/realmroot/lightkite/pkg/common"
	"github.com/realmroot/lightkite/pkg/kube"
	"github.com/realmroot/lightkite/pkg/model"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	authorizationv1 "k8s.io/api/authorization/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgofake "k8s.io/client-go/kubernetes/fake"
	clientgotesting "k8s.io/client-go/testing"
)

func TestGenericResourceHistoryUsesKubernetesAuthorization(t *testing.T) {
	db, user := setupHistoryTestDB(t)
	if err := db.Create(&model.ResourceHistory{
		ClusterName: "prod", ResourceType: "configmaps", ResourceName: "settings", Namespace: "default",
		OperationType: "update", ResourceYAML: "sensitive-history", Success: true, OperatorID: user.ID,
	}).Error; err != nil {
		t.Fatal(err)
	}

	handler := NewGenericResourceHandler[*corev1.ConfigMap, *corev1.ConfigMapList](common.ConfigMaps)
	for _, test := range []struct {
		name       string
		allowed    bool
		query      string
		wantStatus int
	}{
		{name: "allowed", allowed: true, wantStatus: http.StatusOK},
		{name: "denied", allowed: false, wantStatus: http.StatusForbidden},
		{name: "bounded page size", allowed: true, query: "?pageSize=101", wantStatus: http.StatusBadRequest},
	} {
		t.Run(test.name, func(t *testing.T) {
			clientSet := clientgofake.NewSimpleClientset()
			var attributes *authorizationv1.ResourceAttributes
			clientSet.PrependReactor("create", "selfsubjectaccessreviews", func(action clientgotesting.Action) (bool, runtime.Object, error) {
				review := action.(clientgotesting.CreateAction).GetObject().(*authorizationv1.SelfSubjectAccessReview).DeepCopy()
				attributes = review.Spec.ResourceAttributes
				review.Status.Allowed = test.allowed
				review.Status.Reason = "test RBAC decision"
				return true, review, nil
			})

			router := gin.New()
			router.GET("/configmaps/:namespace/:name/history", func(c *gin.Context) {
				c.Set("cluster", &cluster.ClientSet{Name: "prod", K8sClient: &kube.K8sClient{ClientSet: clientSet}})
				handler.ListHistory(c)
			})
			response := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, "/configmaps/default/settings/history"+test.query, nil)
			router.ServeHTTP(response, request)

			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d: %s", response.Code, test.wantStatus, response.Body.String())
			}
			if attributes == nil || attributes.Verb != "get" || attributes.Group != "" ||
				attributes.Resource != "configmaps" || attributes.Namespace != "default" || attributes.Name != "settings" {
				t.Fatalf("authorization attributes = %#v", attributes)
			}
			if test.allowed && response.Code == http.StatusOK {
				var body struct {
					Data []model.ResourceHistory `json:"data"`
				}
				if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
					t.Fatal(err)
				}
				if len(body.Data) != 1 || body.Data[0].Operator == nil || body.Data[0].Operator.Issuer != user.Issuer {
					t.Fatalf("history response = %#v", body.Data)
				}
			}
		})
	}
}

func TestGenericSecretHistoryDoesNotPersistSecretBodies(t *testing.T) {
	_, user := setupHistoryTestDB(t)
	handler := NewGenericResourceHandler[*corev1.Secret, *corev1.SecretList](common.Secrets)
	previous := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "credentials", Namespace: "default"},
		Data:       map[string][]byte{"token": []byte("previous-secret")},
	}
	current := previous.DeepCopy()
	current.Data["token"] = []byte("current-secret")

	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Set("cluster", &cluster.ClientSet{Name: "prod"})
	context.Set("user", user)
	handler.recordHistory(context, "update", previous, current, false, "token must-not-leak")

	var history model.ResourceHistory
	if err := model.DB.First(&history).Error; err != nil {
		t.Fatal(err)
	}
	if history.ResourceYAML != "" || history.PreviousYAML != "" {
		t.Fatalf("secret history persisted bodies: %#v", history)
	}
	if history.ErrorMessage != "Kubernetes Secret operation failed; details omitted" {
		t.Fatalf("secret history persisted raw error: %q", history.ErrorMessage)
	}
}

func TestResourceHistoryIsolatedByStableClusterID(t *testing.T) {
	db, user := setupHistoryTestDB(t)
	for _, history := range []model.ResourceHistory{
		{ClusterID: 11, ClusterName: "reused", ResourceType: "configmaps", ResourceName: "settings", Namespace: "default", OperationType: "update", ResourceYAML: "old-cluster", OperatorID: user.ID},
		{ClusterID: 22, ClusterName: "reused", ResourceType: "configmaps", ResourceName: "settings", Namespace: "default", OperationType: "update", ResourceYAML: "current-cluster", OperatorID: user.ID},
	} {
		if err := db.Create(&history).Error; err != nil {
			t.Fatal(err)
		}
	}
	clientSet := clientgofake.NewSimpleClientset()
	clientSet.PrependReactor("create", "selfsubjectaccessreviews", func(action clientgotesting.Action) (bool, runtime.Object, error) {
		review := action.(clientgotesting.CreateAction).GetObject().(*authorizationv1.SelfSubjectAccessReview).DeepCopy()
		review.Status.Allowed = true
		return true, review, nil
	})
	handler := NewGenericResourceHandler[*corev1.ConfigMap, *corev1.ConfigMapList](common.ConfigMaps)
	router := gin.New()
	router.GET("/history", func(c *gin.Context) {
		c.Params = gin.Params{{Key: "namespace", Value: "default"}, {Key: "name", Value: "settings"}}
		c.Set("cluster", &cluster.ClientSet{ClusterID: 22, Name: "reused", K8sClient: &kube.K8sClient{ClientSet: clientSet}})
		handler.ListHistory(c)
	})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/history", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", response.Code, response.Body.String())
	}
	var body struct {
		Data []model.ResourceHistory `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Data) != 1 || body.Data[0].ResourceYAML != "current-cluster" {
		t.Fatalf("history = %#v, want current cluster only", body.Data)
	}
}

func setupHistoryTestDB(t *testing.T) (*gorm.DB, model.User) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	originalDB := model.DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.ResourceHistory{}); err != nil {
		t.Fatal(err)
	}
	model.DB = db
	t.Cleanup(func() { model.DB = originalDB })
	user := model.User{Issuer: "https://issuer.example.test", Sub: "alice", Username: "alice"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	return db, user
}
