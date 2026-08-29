package resources

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/realmroot/lightkite/pkg/cluster"
	"github.com/realmroot/lightkite/pkg/kube"
	"github.com/realmroot/lightkite/pkg/model"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type applyCreateErrorClient struct {
	client.Client
	err error
}

type applyCreateOnlyClient struct {
	client.Client
}

func (c *applyCreateOnlyClient) Get(_ context.Context, key client.ObjectKey, _ client.Object, _ ...client.GetOption) error {
	return apierrors.NewNotFound(schema.GroupResource{Resource: "secrets"}, key.Name)
}

func (c *applyCreateOnlyClient) Create(context.Context, client.Object, ...client.CreateOption) error {
	return nil
}

func (c *applyCreateErrorClient) Create(context.Context, client.Object, ...client.CreateOption) error {
	return c.err
}

func TestApplyResourceUsesKubernetesAuthorizationAndRESTMapping(t *testing.T) {
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
	user := model.User{Issuer: "https://identity.example.test", Sub: "alice", Username: "alice"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}

	coreVersion := schema.GroupVersion{Version: "v1"}
	ingressVersion := schema.GroupVersion{Group: "networking.k8s.io", Version: "v1"}
	nodeGVK := coreVersion.WithKind("Node")
	secretGVK := coreVersion.WithKind("Secret")
	ingressGVK := ingressVersion.WithKind("Ingress")
	mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{coreVersion, ingressVersion})
	mapper.AddSpecific(nodeGVK, coreVersion.WithResource("nodes"), coreVersion.WithResource("node"), meta.RESTScopeRoot)
	mapper.AddSpecific(secretGVK, coreVersion.WithResource("secrets"), coreVersion.WithResource("secret"), meta.RESTScopeNamespace)
	mapper.AddSpecific(ingressGVK, ingressVersion.WithResource("ingresses"), ingressVersion.WithResource("ingress"), meta.RESTScopeNamespace)
	scheme := runtime.NewScheme()
	scheme.AddKnownTypeWithName(nodeGVK, &unstructured.Unstructured{})
	scheme.AddKnownTypeWithName(ingressGVK, &unstructured.Unstructured{})
	baseClient := fake.NewClientBuilder().WithScheme(scheme).WithRESTMapper(mapper).Build()

	perform := func(k8sClient client.Client, manifest string) *httptest.ResponseRecorder {
		body, err := json.Marshal(ApplyResourceRequest{YAML: manifest})
		if err != nil {
			t.Fatal(err)
		}
		router := gin.New()
		router.POST("/apply", func(c *gin.Context) {
			c.Set("cluster", &cluster.ClientSet{Name: "prod", K8sClient: &kube.K8sClient{Client: k8sClient}})
			c.Set("user", user)
			NewResourceApplyHandler().ApplyResource(c)
		})
		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/apply", strings.NewReader(string(body)))
		request.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(response, request)
		return response
	}

	t.Run("uses mapped plural in audit history", func(t *testing.T) {
		response := perform(baseClient, `apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: web
  namespace: default
spec: {}
`)
		if response.Code != http.StatusOK {
			t.Fatalf("status = %d: %s", response.Code, response.Body.String())
		}
		var history model.ResourceHistory
		if err := db.Order("id DESC").First(&history).Error; err != nil {
			t.Fatal(err)
		}
		if history.ResourceType != "ingresses" || history.OperatorID != user.ID || !history.Success {
			t.Fatalf("unexpected history: %#v", history)
		}
	})

	t.Run("normalizes cluster scoped namespace", func(t *testing.T) {
		response := perform(baseClient, `apiVersion: v1
kind: Node
metadata:
  name: worker
  namespace: forged
`)
		if response.Code != http.StatusOK {
			t.Fatalf("status = %d: %s", response.Code, response.Body.String())
		}
		object := &unstructured.Unstructured{}
		object.SetGroupVersionKind(nodeGVK)
		if err := baseClient.Get(context.Background(), client.ObjectKey{Name: "worker"}, object); err != nil {
			t.Fatal(err)
		}
		if object.GetNamespace() != "" {
			t.Fatalf("node namespace = %q, want empty", object.GetNamespace())
		}
	})

	t.Run("preserves Kubernetes forbidden response", func(t *testing.T) {
		denied := &applyCreateErrorClient{
			Client: baseClient,
			err:    apierrors.NewForbidden(schema.GroupResource{Group: "networking.k8s.io", Resource: "ingresses"}, "denied", nil),
		}
		response := perform(denied, `apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: denied
  namespace: default
`)
		if response.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusForbidden, response.Body.String())
		}
		var history model.ResourceHistory
		if err := db.Order("id DESC").First(&history).Error; err != nil {
			t.Fatal(err)
		}
		if history.Success || !strings.Contains(history.ErrorMessage, "forbidden") {
			t.Fatalf("denied operation was not attributed: %#v", history)
		}
	})

	t.Run("does not persist Secret manifest bodies", func(t *testing.T) {
		response := perform(&applyCreateOnlyClient{Client: baseClient}, `apiVersion: v1
kind: Secret
metadata:
  name: credentials
  namespace: default
stringData:
  token: must-not-be-copied
`)
		if response.Code != http.StatusOK {
			t.Fatalf("status = %d: %s", response.Code, response.Body.String())
		}
		var history model.ResourceHistory
		if err := db.Order("id DESC").First(&history).Error; err != nil {
			t.Fatal(err)
		}
		if history.ResourceType != "secrets" || history.ResourceYAML != "" || history.PreviousYAML != "" {
			t.Fatalf("secret history persisted manifest: %#v", history)
		}
	})
}
