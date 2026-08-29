package resources

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/zxh326/kite/pkg/cluster"
	"github.com/zxh326/kite/pkg/common"
	"github.com/zxh326/kite/pkg/kube"
	"github.com/zxh326/kite/pkg/middleware"
	"github.com/zxh326/kite/pkg/model"
	authorizationv1 "k8s.io/api/authorization/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	clientgofake "k8s.io/client-go/kubernetes/fake"
	clientgotesting "k8s.io/client-go/testing"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

const widgetCRDName = "widgets.example.com"

var widgetGVK = schema.GroupVersionKind{Group: "example.com", Version: "v1", Kind: "Widget"}

var widgetListGVK = schema.GroupVersionKind{Group: "example.com", Version: "v1", Kind: "WidgetList"}

type crAPITestConfig struct {
	user                 model.User
	write                bool
	clusterAObjects      []client.Object
	clusterBObjects      []client.Object
	clusterAInterceptors interceptor.Funcs
	clusterBInterceptors interceptor.Funcs
	denyHistory          bool
}

type crAPITestFixture struct {
	router  *gin.Engine
	clients map[string]client.Client
	reviews *[]*authorizationv1.ResourceAttributes
}

type crAPITestClusterProvider map[string]*cluster.ClientSet

func (p crAPITestClusterProvider) GetClientSet(clusterName, _ string) (*cluster.ClientSet, error) {
	if clusterName == "" {
		clusterName = "cluster-a"
	}
	clientSet, ok := p[clusterName]
	if !ok {
		return nil, fmt.Errorf("cluster not found: %s", clusterName)
	}
	return clientSet, nil
}

func newCRAPITestFixture(t *testing.T, config crAPITestConfig) *crAPITestFixture {
	t.Helper()

	scheme := runtime.NewScheme()
	if err := apiextensionsv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}

	if config.user.Username == "" {
		config.user.Username = "operator"
	}
	if config.write {
		setupResourceAPITestDB(t, config.user)
	}

	newClient := func(objects []client.Object, funcs interceptor.Funcs) client.Client {
		return fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(objects...).
			WithReturnManagedFields().
			WithInterceptorFuncs(funcs).
			Build()
	}
	clusterAClient := newClient(config.clusterAObjects, config.clusterAInterceptors)
	clusterBClient := newClient(config.clusterBObjects, config.clusterBInterceptors)
	reviews := make([]*authorizationv1.ResourceAttributes, 0)
	newAuthorizationClient := func() *clientgofake.Clientset {
		clientSet := clientgofake.NewSimpleClientset()
		clientSet.PrependReactor("create", "selfsubjectaccessreviews", func(action clientgotesting.Action) (bool, runtime.Object, error) {
			review := action.(clientgotesting.CreateAction).GetObject().(*authorizationv1.SelfSubjectAccessReview).DeepCopy()
			reviews = append(reviews, review.Spec.ResourceAttributes.DeepCopy())
			review.Status.Allowed = !config.denyHistory
			review.Status.Reason = "test RBAC decision"
			return true, review, nil
		})
		return clientSet
	}
	clientSets := crAPITestClusterProvider{
		"cluster-a": {
			Name: "cluster-a",
			K8sClient: &kube.K8sClient{
				Client:    clusterAClient,
				ClientSet: newAuthorizationClient(),
			},
		},
		"cluster-b": {
			Name: "cluster-b",
			K8sClient: &kube.K8sClient{
				Client:    clusterBClient,
				ClientSet: newAuthorizationClient(),
			},
		},
	}

	oldHandlers := handlers
	t.Cleanup(func() {
		handlers = oldHandlers
	})

	oldGinMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() {
		gin.SetMode(oldGinMode)
	})
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("user", config.user)
		c.Set("oidc-id-token", "test-id-token")
	})

	api := router.Group("/api/v1")
	for _, group := range []*gin.RouterGroup{
		api.Group(""),
		api.Group("/_clusters/:cluster"),
	} {
		group.Use(middleware.ClusterMiddleware(clientSets))
		RegisterRoutes(group)
	}

	return &crAPITestFixture{
		router: router,
		clients: map[string]client.Client{
			"cluster-a": clusterAClient,
			"cluster-b": clusterBClient,
		},
		reviews: &reviews,
	}
}

func performCRAPIRequest(t *testing.T, fixture *crAPITestFixture, method, target, body string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()

	response := httptest.NewRecorder()
	request := httptest.NewRequest(method, target, bytes.NewBufferString(body))
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	fixture.router.ServeHTTP(response, request)
	return response
}

func decodeCRAPIResponse[T any](t *testing.T, response *httptest.ResponseRecorder) T {
	t.Helper()

	var value T
	if err := json.Unmarshal(response.Body.Bytes(), &value); err != nil {
		t.Fatalf("decode response: %v; body=%s", err, response.Body.String())
	}
	return value
}

func newWidgetCRD(scope apiextensionsv1.ResourceScope) *apiextensionsv1.CustomResourceDefinition {
	return &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: widgetCRDName},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: "example.com",
			Names: apiextensionsv1.CustomResourceDefinitionNames{
				Plural:   "widgets",
				Singular: "widget",
				Kind:     "Widget",
				ListKind: "WidgetList",
			},
			Scope: scope,
			Versions: []apiextensionsv1.CustomResourceDefinitionVersion{{
				Name:    "v1",
				Served:  true,
				Storage: true,
			}},
		},
	}
}

func newWidget(name, namespace string, labels map[string]string) *unstructured.Unstructured {
	widget := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "example.com/v1",
		"kind":       "Widget",
		"metadata": map[string]interface{}{
			"name":      name,
			"namespace": namespace,
		},
		"spec": map[string]interface{}{
			"size": "small",
		},
	}}
	widget.SetGroupVersionKind(widgetGVK)
	widget.SetLabels(labels)
	return widget
}

func newWidgetUser() model.User {
	return model.User{Username: "alice"}
}

func TestCRAPIList(t *testing.T) {
	user := newWidgetUser()

	t.Run("default all and namespace routes", func(t *testing.T) {
		fixture := newCRAPITestFixture(t, crAPITestConfig{
			user: user,
			clusterAObjects: []client.Object{
				newWidgetCRD(apiextensionsv1.NamespaceScoped),
				newWidget("default-widget", "default", nil),
				newWidget("team-widget", "team-a", nil),
			},
		})

		for _, test := range []struct {
			path string
			want []string
		}{
			{path: "/api/v1/widgets.example.com", want: []string{"default/default-widget", "team-a/team-widget"}},
			{path: "/api/v1/widgets.example.com/_all", want: []string{"default/default-widget", "team-a/team-widget"}},
			{path: "/api/v1/widgets.example.com/default", want: []string{"default/default-widget"}},
		} {
			response := performCRAPIRequest(t, fixture, http.MethodGet, test.path, "", nil)
			if response.Code != http.StatusOK {
				t.Fatalf("GET %s returned %d, want %d; body=%s", test.path, response.Code, http.StatusOK, response.Body.String())
			}
			list := decodeCRAPIResponse[unstructured.UnstructuredList](t, response)
			got := make([]string, len(list.Items))
			for i := range list.Items {
				got[i] = list.Items[i].GetNamespace() + "/" + list.Items[i].GetName()
			}
			sort.Strings(got)
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("GET %s returned %v, want %v", test.path, got, test.want)
			}
		}
	})

	t.Run("passes namespace and label selector", func(t *testing.T) {
		var gotOptions client.ListOptions
		fixture := newCRAPITestFixture(t, crAPITestConfig{
			user: user,
			clusterAObjects: []client.Object{
				newWidgetCRD(apiextensionsv1.NamespaceScoped),
				newWidget("web", "default", map[string]string{"app": "web"}),
				newWidget("worker", "default", map[string]string{"app": "worker"}),
			},
			clusterAInterceptors: interceptor.Funcs{
				List: func(ctx context.Context, underlying client.WithWatch, list client.ObjectList, options ...client.ListOption) error {
					if list.GetObjectKind().GroupVersionKind() == widgetListGVK {
						gotOptions = *(&client.ListOptions{}).ApplyOptions(options)
					}
					return underlying.List(ctx, list, options...)
				},
			},
		})

		response := performCRAPIRequest(t, fixture, http.MethodGet, "/api/v1/widgets.example.com/default?labelSelector=app%3Dweb", "", nil)
		if response.Code != http.StatusOK {
			t.Fatalf("list returned %d, want %d; body=%s", response.Code, http.StatusOK, response.Body.String())
		}
		list := decodeCRAPIResponse[unstructured.UnstructuredList](t, response)
		if len(list.Items) != 1 || list.Items[0].GetName() != "web" {
			t.Fatalf("list items = %#v, want web", list.Items)
		}
		if gotOptions.Namespace != "default" {
			t.Fatalf("list namespace = %q, want default", gotOptions.Namespace)
		}
		if gotOptions.LabelSelector == nil || gotOptions.LabelSelector.String() != "app=web" {
			t.Fatalf("label selector = %v, want app=web", gotOptions.LabelSelector)
		}
	})

	t.Run("invalid label selector", func(t *testing.T) {
		fixture := newCRAPITestFixture(t, crAPITestConfig{
			user:            user,
			clusterAObjects: []client.Object{newWidgetCRD(apiextensionsv1.NamespaceScoped)},
		})
		response := performCRAPIRequest(t, fixture, http.MethodGet, "/api/v1/widgets.example.com/default?labelSelector=app%20in%20%28", "", nil)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("invalid selector returned %d, want %d; body=%s", response.Code, http.StatusBadRequest, response.Body.String())
		}
	})

	t.Run("CRD not found", func(t *testing.T) {
		fixture := newCRAPITestFixture(t, crAPITestConfig{user: user})
		response := performCRAPIRequest(t, fixture, http.MethodGet, "/api/v1/widgets.example.com/default", "", nil)
		if response.Code != http.StatusNotFound {
			t.Fatalf("missing CRD returned %d, want %d; body=%s", response.Code, http.StatusNotFound, response.Body.String())
		}
		body := decodeCRAPIResponse[map[string]string](t, response)
		if body["error"] != "CustomResourceDefinition not found" {
			t.Fatalf("error = %q, want CustomResourceDefinition not found", body["error"])
		}
	})

	t.Run("CRD get error", func(t *testing.T) {
		fixture := newCRAPITestFixture(t, crAPITestConfig{
			user: user,
			clusterAInterceptors: interceptor.Funcs{
				Get: func(_ context.Context, _ client.WithWatch, _ client.ObjectKey, object client.Object, _ ...client.GetOption) error {
					if _, ok := object.(*apiextensionsv1.CustomResourceDefinition); ok {
						return errors.New("CRD get failed")
					}
					return nil
				},
			},
		})
		response := performCRAPIRequest(t, fixture, http.MethodGet, "/api/v1/widgets.example.com/default", "", nil)
		if response.Code != http.StatusInternalServerError {
			t.Fatalf("CRD get error returned %d, want %d; body=%s", response.Code, http.StatusInternalServerError, response.Body.String())
		}
	})

	t.Run("custom resource list error", func(t *testing.T) {
		fixture := newCRAPITestFixture(t, crAPITestConfig{
			user:            user,
			clusterAObjects: []client.Object{newWidgetCRD(apiextensionsv1.NamespaceScoped)},
			clusterAInterceptors: interceptor.Funcs{
				List: func(_ context.Context, _ client.WithWatch, list client.ObjectList, _ ...client.ListOption) error {
					if list.GetObjectKind().GroupVersionKind() == widgetListGVK {
						return errors.New("widget list failed")
					}
					return nil
				},
			},
		})
		response := performCRAPIRequest(t, fixture, http.MethodGet, "/api/v1/widgets.example.com/default", "", nil)
		if response.Code != http.StatusInternalServerError {
			t.Fatalf("list error returned %d, want %d; body=%s", response.Code, http.StatusInternalServerError, response.Body.String())
		}
	})

	t.Run("cluster scoped resource rejects namespace", func(t *testing.T) {
		fixture := newCRAPITestFixture(t, crAPITestConfig{
			user:            user,
			clusterAObjects: []client.Object{newWidgetCRD(apiextensionsv1.ClusterScoped)},
		})
		response := performCRAPIRequest(t, fixture, http.MethodGet, "/api/v1/widgets.example.com/default", "", nil)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("cluster scoped namespace returned %d, want %d; body=%s", response.Code, http.StatusBadRequest, response.Body.String())
		}
	})
}

func TestCRAPIGet(t *testing.T) {
	user := newWidgetUser()

	t.Run("gets namespaced resource and removes server metadata", func(t *testing.T) {
		widget := newWidget("target", "default", nil)
		widget.SetAnnotations(map[string]string{
			common.KubectlAnnotation: "removed",
			"keep":                   "value",
		})
		fixture := newCRAPITestFixture(t, crAPITestConfig{
			user: user,
			clusterAObjects: []client.Object{
				newWidgetCRD(apiextensionsv1.NamespaceScoped),
				widget,
			},
			clusterAInterceptors: interceptor.Funcs{
				Get: func(ctx context.Context, underlying client.WithWatch, key client.ObjectKey, object client.Object, options ...client.GetOption) error {
					if err := underlying.Get(ctx, key, object, options...); err != nil {
						return err
					}
					if cr, ok := object.(*unstructured.Unstructured); ok {
						cr.SetManagedFields([]metav1.ManagedFieldsEntry{{Manager: "kubectl"}})
					}
					return nil
				},
			},
		})

		response := performCRAPIRequest(t, fixture, http.MethodGet, "/api/v1/widgets.example.com/default/target", "", nil)
		if response.Code != http.StatusOK {
			t.Fatalf("get returned %d, want %d; body=%s", response.Code, http.StatusOK, response.Body.String())
		}
		got := decodeCRAPIResponse[unstructured.Unstructured](t, response)
		if got.GetName() != "target" || got.GetNamespace() != "default" || got.GroupVersionKind() != widgetGVK {
			t.Fatalf("get returned %s %s/%s, want %s default/target", got.GroupVersionKind(), got.GetNamespace(), got.GetName(), widgetGVK)
		}
		if len(got.GetManagedFields()) != 0 {
			t.Fatalf("managedFields = %#v, want empty", got.GetManagedFields())
		}
		if _, ok := got.GetAnnotations()[common.KubectlAnnotation]; ok {
			t.Fatalf("annotations still contain %q", common.KubectlAnnotation)
		}
		if got.GetAnnotations()["keep"] != "value" {
			t.Fatalf("annotations = %#v, want keep=value", got.GetAnnotations())
		}
	})

	t.Run("gets cluster scoped resource through all namespace route", func(t *testing.T) {
		fixture := newCRAPITestFixture(t, crAPITestConfig{
			user: user,
			clusterAObjects: []client.Object{
				newWidgetCRD(apiextensionsv1.ClusterScoped),
				newWidget("global", "", nil),
			},
		})
		response := performCRAPIRequest(t, fixture, http.MethodGet, "/api/v1/widgets.example.com/_all/global", "", nil)
		if response.Code != http.StatusOK {
			t.Fatalf("cluster scoped get returned %d, want %d; body=%s", response.Code, http.StatusOK, response.Body.String())
		}
		got := decodeCRAPIResponse[unstructured.Unstructured](t, response)
		if got.GetName() != "global" || got.GetNamespace() != "" {
			t.Fatalf("cluster scoped get returned %q/%q, want global with empty namespace", got.GetNamespace(), got.GetName())
		}
	})

	t.Run("namespaced resource rejects all namespace get", func(t *testing.T) {
		fixture := newCRAPITestFixture(t, crAPITestConfig{
			user:            user,
			clusterAObjects: []client.Object{newWidgetCRD(apiextensionsv1.NamespaceScoped)},
		})
		response := performCRAPIRequest(t, fixture, http.MethodGet, "/api/v1/widgets.example.com/_all/target", "", nil)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("all namespace get returned %d, want %d; body=%s", response.Code, http.StatusBadRequest, response.Body.String())
		}
	})

	t.Run("custom resource not found", func(t *testing.T) {
		fixture := newCRAPITestFixture(t, crAPITestConfig{
			user:            user,
			clusterAObjects: []client.Object{newWidgetCRD(apiextensionsv1.NamespaceScoped)},
		})
		response := performCRAPIRequest(t, fixture, http.MethodGet, "/api/v1/widgets.example.com/default/missing", "", nil)
		if response.Code != http.StatusNotFound {
			t.Fatalf("missing resource returned %d, want %d; body=%s", response.Code, http.StatusNotFound, response.Body.String())
		}
		body := decodeCRAPIResponse[map[string]string](t, response)
		if body["error"] != "Custom resource not found" {
			t.Fatalf("error = %q, want Custom resource not found", body["error"])
		}
	})

	t.Run("custom resource get error", func(t *testing.T) {
		fixture := newCRAPITestFixture(t, crAPITestConfig{
			user:            user,
			clusterAObjects: []client.Object{newWidgetCRD(apiextensionsv1.NamespaceScoped)},
			clusterAInterceptors: interceptor.Funcs{
				Get: func(ctx context.Context, underlying client.WithWatch, key client.ObjectKey, object client.Object, options ...client.GetOption) error {
					if _, ok := object.(*unstructured.Unstructured); ok {
						return errors.New("widget get failed")
					}
					return underlying.Get(ctx, key, object, options...)
				},
			},
		})
		response := performCRAPIRequest(t, fixture, http.MethodGet, "/api/v1/widgets.example.com/default/target", "", nil)
		if response.Code != http.StatusInternalServerError {
			t.Fatalf("get error returned %d, want %d; body=%s", response.Code, http.StatusInternalServerError, response.Body.String())
		}
	})
}

func TestCRAPIUpdate(t *testing.T) {
	user := newWidgetUser()

	t.Run("uses URL identity and preserves stored metadata", func(t *testing.T) {
		widget := newWidget("target", "default", nil)
		widget.SetResourceVersion("7")
		widget.SetUID(types.UID("stored-uid"))
		var sentToClient *unstructured.Unstructured
		fixture := newCRAPITestFixture(t, crAPITestConfig{
			user:  user,
			write: true,
			clusterAObjects: []client.Object{
				newWidgetCRD(apiextensionsv1.NamespaceScoped),
				widget,
			},
			clusterAInterceptors: interceptor.Funcs{
				Update: func(ctx context.Context, underlying client.WithWatch, object client.Object, options ...client.UpdateOption) error {
					if cr, ok := object.(*unstructured.Unstructured); ok {
						sentToClient = cr.DeepCopy()
					}
					return underlying.Update(ctx, object, options...)
				},
			},
		})
		body := `{
			"apiVersion":"other.example.com/v9",
			"kind":"Other",
			"metadata":{"name":"body-name","namespace":"team-a","resourceVersion":"99","uid":"body-uid"},
			"spec":{"size":"large"}
		}`
		response := performCRAPIRequest(t, fixture, http.MethodPut, "/api/v1/widgets.example.com/default/target", body, nil)
		if response.Code != http.StatusOK {
			t.Fatalf("update returned %d, want %d; body=%s", response.Code, http.StatusOK, response.Body.String())
		}
		updated := decodeCRAPIResponse[unstructured.Unstructured](t, response)
		if updated.GetName() != "target" || updated.GetNamespace() != "default" {
			t.Fatalf("updated identity = %s/%s, want default/target", updated.GetNamespace(), updated.GetName())
		}
		if updated.GroupVersionKind() != widgetGVK {
			t.Fatalf("updated GVK = %s, want %s", updated.GroupVersionKind(), widgetGVK)
		}
		if sentToClient == nil {
			t.Fatal("update did not reach custom resource client")
		}
		if sentToClient.GetName() != "target" || sentToClient.GetNamespace() != "default" || sentToClient.GroupVersionKind() != widgetGVK {
			t.Fatalf("client received %s %s/%s, want %s default/target", sentToClient.GroupVersionKind(), sentToClient.GetNamespace(), sentToClient.GetName(), widgetGVK)
		}
		if sentToClient.GetResourceVersion() != "7" || sentToClient.GetUID() != types.UID("stored-uid") {
			t.Fatalf("client received resourceVersion/UID = %q/%q, want 7/stored-uid", sentToClient.GetResourceVersion(), sentToClient.GetUID())
		}
		size, found, err := unstructured.NestedString(updated.Object, "spec", "size")
		if err != nil || !found || size != "large" {
			t.Fatalf("updated spec.size = %q, found=%t, err=%v; want large", size, found, err)
		}

		stored := &unstructured.Unstructured{}
		stored.SetGroupVersionKind(widgetGVK)
		if err := fixture.clients["cluster-a"].Get(context.Background(), types.NamespacedName{Namespace: "default", Name: "target"}, stored); err != nil {
			t.Fatalf("get stored widget: %v", err)
		}
		storedSize, _, err := unstructured.NestedString(stored.Object, "spec", "size")
		if err != nil || storedSize != "large" {
			t.Fatalf("stored spec.size = %q, err=%v; want large", storedSize, err)
		}
	})

	t.Run("invalid JSON", func(t *testing.T) {
		fixture := newCRAPITestFixture(t, crAPITestConfig{
			user:  user,
			write: true,
			clusterAObjects: []client.Object{
				newWidgetCRD(apiextensionsv1.NamespaceScoped),
				newWidget("target", "default", nil),
			},
		})
		response := performCRAPIRequest(t, fixture, http.MethodPut, "/api/v1/widgets.example.com/default/target", `{`, nil)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("invalid JSON returned %d, want %d; body=%s", response.Code, http.StatusBadRequest, response.Body.String())
		}
	})

	t.Run("custom resource not found", func(t *testing.T) {
		fixture := newCRAPITestFixture(t, crAPITestConfig{
			user:            user,
			write:           true,
			clusterAObjects: []client.Object{newWidgetCRD(apiextensionsv1.NamespaceScoped)},
		})
		response := performCRAPIRequest(t, fixture, http.MethodPut, "/api/v1/widgets.example.com/default/missing", `{}`, nil)
		if response.Code != http.StatusNotFound {
			t.Fatalf("missing resource returned %d, want %d; body=%s", response.Code, http.StatusNotFound, response.Body.String())
		}
	})

	t.Run("update error", func(t *testing.T) {
		fixture := newCRAPITestFixture(t, crAPITestConfig{
			user:  user,
			write: true,
			clusterAObjects: []client.Object{
				newWidgetCRD(apiextensionsv1.NamespaceScoped),
				newWidget("target", "default", nil),
			},
			clusterAInterceptors: interceptor.Funcs{
				Update: func(_ context.Context, _ client.WithWatch, object client.Object, _ ...client.UpdateOption) error {
					if _, ok := object.(*unstructured.Unstructured); ok {
						return errors.New("widget update failed")
					}
					return nil
				},
			},
		})
		response := performCRAPIRequest(t, fixture, http.MethodPut, "/api/v1/widgets.example.com/default/target", `{"apiVersion":"example.com/v1","kind":"Widget","spec":{"size":"large"}}`, nil)
		if response.Code != http.StatusInternalServerError {
			t.Fatalf("update error returned %d, want %d; body=%s", response.Code, http.StatusInternalServerError, response.Body.String())
		}
	})

	t.Run("namespaced resource rejects all namespace update", func(t *testing.T) {
		fixture := newCRAPITestFixture(t, crAPITestConfig{
			user:            user,
			write:           true,
			clusterAObjects: []client.Object{newWidgetCRD(apiextensionsv1.NamespaceScoped)},
		})
		response := performCRAPIRequest(t, fixture, http.MethodPut, "/api/v1/widgets.example.com/_all/target", `{}`, nil)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("all namespace update returned %d, want %d; body=%s", response.Code, http.StatusBadRequest, response.Body.String())
		}
	})
}

func TestCRAPIDelete(t *testing.T) {
	user := newWidgetUser()

	t.Run("uses background propagation and deletes resource", func(t *testing.T) {
		var gotOptions client.DeleteOptions
		getCalls := 0
		fixture := newCRAPITestFixture(t, crAPITestConfig{
			user:  user,
			write: true,
			clusterAObjects: []client.Object{
				newWidgetCRD(apiextensionsv1.NamespaceScoped),
				newWidget("target", "default", nil),
			},
			clusterAInterceptors: interceptor.Funcs{
				Get: func(ctx context.Context, underlying client.WithWatch, key client.ObjectKey, object client.Object, options ...client.GetOption) error {
					if _, ok := object.(*unstructured.Unstructured); ok {
						getCalls++
					}
					return underlying.Get(ctx, key, object, options...)
				},
				Delete: func(ctx context.Context, underlying client.WithWatch, object client.Object, options ...client.DeleteOption) error {
					gotOptions = *(&client.DeleteOptions{}).ApplyOptions(options)
					return underlying.Delete(ctx, object, options...)
				},
			},
		})

		response := performCRAPIRequest(t, fixture, http.MethodDelete, "/api/v1/widgets.example.com/default/target?wait=false", "", nil)
		if response.Code != http.StatusOK {
			t.Fatalf("delete returned %d, want %d; body=%s", response.Code, http.StatusOK, response.Body.String())
		}
		body := decodeCRAPIResponse[map[string]string](t, response)
		if body["message"] != "Custom resource deleted successfully" {
			t.Fatalf("message = %q, want Custom resource deleted successfully", body["message"])
		}
		if gotOptions.PropagationPolicy == nil || *gotOptions.PropagationPolicy != metav1.DeletePropagationBackground {
			t.Fatalf("propagation policy = %v, want Background", gotOptions.PropagationPolicy)
		}
		if gotOptions.GracePeriodSeconds != nil {
			t.Fatalf("grace period = %v, want nil", gotOptions.GracePeriodSeconds)
		}
		if getCalls != 1 {
			t.Fatalf("custom resource Get calls = %d, want 1 with wait=false", getCalls)
		}

		deleted := &unstructured.Unstructured{}
		deleted.SetGroupVersionKind(widgetGVK)
		err := fixture.clients["cluster-a"].Get(context.Background(), types.NamespacedName{Namespace: "default", Name: "target"}, deleted)
		if !k8serrors.IsNotFound(err) {
			t.Fatalf("get deleted widget error = %v, want not found", err)
		}
	})

	t.Run("force uses zero grace period without waiting", func(t *testing.T) {
		var gotOptions client.DeleteOptions
		getCalls := 0
		fixture := newCRAPITestFixture(t, crAPITestConfig{
			user:  user,
			write: true,
			clusterAObjects: []client.Object{
				newWidgetCRD(apiextensionsv1.NamespaceScoped),
				newWidget("target", "default", nil),
			},
			clusterAInterceptors: interceptor.Funcs{
				Get: func(ctx context.Context, underlying client.WithWatch, key client.ObjectKey, object client.Object, options ...client.GetOption) error {
					if _, ok := object.(*unstructured.Unstructured); ok {
						getCalls++
					}
					return underlying.Get(ctx, key, object, options...)
				},
				Delete: func(ctx context.Context, underlying client.WithWatch, object client.Object, options ...client.DeleteOption) error {
					gotOptions = *(&client.DeleteOptions{}).ApplyOptions(options)
					return underlying.Delete(ctx, object, options...)
				},
			},
		})

		response := performCRAPIRequest(t, fixture, http.MethodDelete, "/api/v1/widgets.example.com/default/target?force=true&wait=false", "", nil)
		if response.Code != http.StatusOK {
			t.Fatalf("force delete returned %d, want %d; body=%s", response.Code, http.StatusOK, response.Body.String())
		}
		if gotOptions.PropagationPolicy == nil || *gotOptions.PropagationPolicy != metav1.DeletePropagationBackground {
			t.Fatalf("propagation policy = %v, want Background", gotOptions.PropagationPolicy)
		}
		if gotOptions.GracePeriodSeconds == nil || *gotOptions.GracePeriodSeconds != 0 {
			t.Fatalf("grace period = %v, want 0", gotOptions.GracePeriodSeconds)
		}
		if getCalls != 1 {
			t.Fatalf("custom resource Get calls = %d, want 1 with wait=false", getCalls)
		}
	})

	t.Run("custom resource not found", func(t *testing.T) {
		fixture := newCRAPITestFixture(t, crAPITestConfig{
			user:            user,
			write:           true,
			clusterAObjects: []client.Object{newWidgetCRD(apiextensionsv1.NamespaceScoped)},
		})
		response := performCRAPIRequest(t, fixture, http.MethodDelete, "/api/v1/widgets.example.com/default/missing?wait=false", "", nil)
		if response.Code != http.StatusNotFound {
			t.Fatalf("missing resource returned %d, want %d; body=%s", response.Code, http.StatusNotFound, response.Body.String())
		}
	})

	t.Run("custom resource get error", func(t *testing.T) {
		fixture := newCRAPITestFixture(t, crAPITestConfig{
			user:            user,
			write:           true,
			clusterAObjects: []client.Object{newWidgetCRD(apiextensionsv1.NamespaceScoped)},
			clusterAInterceptors: interceptor.Funcs{
				Get: func(ctx context.Context, underlying client.WithWatch, key client.ObjectKey, object client.Object, options ...client.GetOption) error {
					if _, ok := object.(*unstructured.Unstructured); ok {
						return errors.New("widget get failed")
					}
					return underlying.Get(ctx, key, object, options...)
				},
			},
		})
		response := performCRAPIRequest(t, fixture, http.MethodDelete, "/api/v1/widgets.example.com/default/target?wait=false", "", nil)
		if response.Code != http.StatusInternalServerError {
			t.Fatalf("get error returned %d, want %d; body=%s", response.Code, http.StatusInternalServerError, response.Body.String())
		}
	})

	t.Run("delete error", func(t *testing.T) {
		fixture := newCRAPITestFixture(t, crAPITestConfig{
			user:  user,
			write: true,
			clusterAObjects: []client.Object{
				newWidgetCRD(apiextensionsv1.NamespaceScoped),
				newWidget("target", "default", nil),
			},
			clusterAInterceptors: interceptor.Funcs{
				Delete: func(_ context.Context, _ client.WithWatch, _ client.Object, _ ...client.DeleteOption) error {
					return errors.New("widget delete failed")
				},
			},
		})
		response := performCRAPIRequest(t, fixture, http.MethodDelete, "/api/v1/widgets.example.com/default/target?wait=false", "", nil)
		if response.Code != http.StatusInternalServerError {
			t.Fatalf("delete error returned %d, want %d; body=%s", response.Code, http.StatusInternalServerError, response.Body.String())
		}
	})
}

func TestCRAPIHistoryUsesCustomResourceAuthorization(t *testing.T) {
	for _, test := range []struct {
		name       string
		deny       bool
		wantStatus int
	}{
		{name: "allowed", wantStatus: http.StatusOK},
		{name: "denied", deny: true, wantStatus: http.StatusForbidden},
	} {
		t.Run(test.name, func(t *testing.T) {
			user := model.User{Issuer: "https://issuer.example.test", Sub: "alice", Username: "alice"}
			fixture := newCRAPITestFixture(t, crAPITestConfig{
				user:            user,
				write:           true,
				denyHistory:     test.deny,
				clusterAObjects: []client.Object{newWidgetCRD(apiextensionsv1.NamespaceScoped)},
			})
			response := performCRAPIRequest(t, fixture, http.MethodGet, "/api/v1/widgets.example.com/default/target/history", "", nil)
			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d: %s", response.Code, test.wantStatus, response.Body.String())
			}
			if fixture.reviews == nil || len(*fixture.reviews) != 1 {
				t.Fatalf("authorization reviews = %#v", fixture.reviews)
			}
			attributes := (*fixture.reviews)[0]
			if attributes.Verb != "get" || attributes.Group != "example.com" || attributes.Resource != "widgets" ||
				attributes.Namespace != "default" || attributes.Name != "target" {
				t.Fatalf("authorization attributes = %#v", attributes)
			}
		})
	}
}

func TestCRAPIClusterSelection(t *testing.T) {
	fixture := newCRAPITestFixture(t, crAPITestConfig{
		user: newWidgetUser(),
		clusterAObjects: []client.Object{
			newWidgetCRD(apiextensionsv1.NamespaceScoped),
			newWidget("from-a", "default", nil),
		},
		clusterBObjects: []client.Object{
			newWidgetCRD(apiextensionsv1.NamespaceScoped),
			newWidget("from-b", "default", nil),
		},
	})

	for _, test := range []struct {
		name       string
		path       string
		headers    map[string]string
		wantWidget string
	}{
		{name: "default cluster", path: "/api/v1/widgets.example.com/default", wantWidget: "from-a"},
		{name: "query selects cluster b", path: "/api/v1/widgets.example.com/default?x-cluster-name=cluster-b", wantWidget: "from-b"},
		{name: "header overrides query", path: "/api/v1/widgets.example.com/default?x-cluster-name=cluster-a", headers: map[string]string{middleware.ClusterNameHeader: "cluster-b"}, wantWidget: "from-b"},
		{name: "path overrides header and query", path: "/api/v1/_clusters/cluster-a/widgets.example.com/default?x-cluster-name=cluster-b", headers: map[string]string{middleware.ClusterNameHeader: "cluster-b"}, wantWidget: "from-a"},
		{name: "explicit cluster path parses dynamic resource", path: "/api/v1/_clusters/cluster-b/widgets.example.com/default", wantWidget: "from-b"},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := performCRAPIRequest(t, fixture, http.MethodGet, test.path, "", test.headers)
			if response.Code != http.StatusOK {
				t.Fatalf("GET %s returned %d, want %d; body=%s", test.path, response.Code, http.StatusOK, response.Body.String())
			}
			list := decodeCRAPIResponse[unstructured.UnstructuredList](t, response)
			if len(list.Items) != 1 || list.Items[0].GetName() != test.wantWidget || list.Items[0].GetNamespace() != "default" {
				t.Fatalf("GET %s returned %#v, want default/%s", test.path, list.Items, test.wantWidget)
			}
		})
	}

	t.Run("unknown cluster", func(t *testing.T) {
		response := performCRAPIRequest(t, fixture, http.MethodGet, "/api/v1/widgets.example.com/default?x-cluster-name=missing", "", nil)
		if response.Code != http.StatusNotFound {
			t.Fatalf("unknown cluster returned %d, want %d; body=%s", response.Code, http.StatusNotFound, response.Body.String())
		}
		body := decodeCRAPIResponse[map[string]string](t, response)
		if body["error"] != "cluster not found: missing" {
			t.Fatalf("error = %q, want cluster not found: missing", body["error"])
		}
	})
}
