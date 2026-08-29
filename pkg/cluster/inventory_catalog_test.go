package cluster

import (
	"context"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientcmdv1 "k8s.io/client-go/tools/clientcmd/api/v1"
	apisv1alpha1 "sigs.k8s.io/cluster-inventory-api/apis/v1alpha1"
)

func TestInventoryContextFromProfileUsesCredentiallessAccessProvider(t *testing.T) {
	profile := inventoryProfile("https://hub.example.test/clusters/kind-realmroot/kubernetes")

	context, err := inventoryContextFromProfile(profile)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { closeIdleConnections(context.transport) })
	if context.name != "cluster-inventory--cluster-inventory--kind-realmroot" {
		t.Fatalf("context name = %q", context.name)
	}
	if context.displayName != "Kind Realmroot" {
		t.Fatalf("display name = %q", context.displayName)
	}
	if context.config.Host != "https://hub.example.test/clusters/kind-realmroot/kubernetes" {
		t.Fatalf("host = %q", context.config.Host)
	}
	if context.config.BearerToken != "" || context.config.ExecProvider != nil {
		t.Fatal("inventory context persisted a credential")
	}
}

func TestInventoryCatalogClientSetAddsOnlyRequestToken(t *testing.T) {
	profile := inventoryProfile("https://hub.example.test/clusters/kind-realmroot/kubernetes")
	context, err := inventoryContextFromProfile(profile)
	if err != nil {
		t.Fatal(err)
	}
	catalog := &inventoryCatalog{contexts: map[string]*inventoryContext{context.name: context}}
	t.Cleanup(func() { catalog.delete(context.name) })

	clientSet, err := catalog.clientSet(context.name, "request-id-token")
	if err != nil {
		t.Fatal(err)
	}
	if clientSet.Name != context.name || clientSet.ClusterID != 0 {
		t.Fatalf("client set identity = %#v", clientSet)
	}
	if clientSet.K8sClient.Configuration.BearerToken != "request-id-token" {
		t.Fatal("request token was not attached to the request-scoped client")
	}
}

func TestInventoryCatalogWriteUsesRequestIdentityAndAccessProviderPath(t *testing.T) {
	var method, path, authorization string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/clusters/kind-realmroot/kubernetes/api":
			_, _ = w.Write([]byte(`{"apiVersion":"v1","kind":"APIVersions","versions":["v1"]}`))
		case "/clusters/kind-realmroot/kubernetes/apis":
			_, _ = w.Write([]byte(`{"apiVersion":"v1","kind":"APIGroupList","groups":[]}`))
		case "/clusters/kind-realmroot/kubernetes/api/v1":
			_, _ = w.Write([]byte(`{"apiVersion":"v1","kind":"APIResourceList","groupVersion":"v1","resources":[{"name":"configmaps","singularName":"","namespaced":true,"kind":"ConfigMap","verbs":["create"]}]}`))
		default:
			method, path, authorization = r.Method, r.URL.Path, r.Header.Get("Authorization")
			_, _ = w.Write([]byte(`{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"write-smoke","namespace":"default"}}`))
		}
	}))
	t.Cleanup(server.Close)
	profile := inventoryProfile(server.URL + "/clusters/kind-realmroot/kubernetes")
	profile.Status.AccessProviders[0].Cluster.CertificateAuthorityData = pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: server.Certificate().Raw,
	})
	item, err := inventoryContextFromProfile(profile)
	if err != nil {
		t.Fatal(err)
	}
	catalog := &inventoryCatalog{contexts: map[string]*inventoryContext{item.name: item}}
	t.Cleanup(func() { catalog.delete(item.name) })
	clientSet, err := catalog.clientSet(item.name, "request-id-token")
	if err != nil {
		t.Fatal(err)
	}

	err = clientSet.K8sClient.Create(context.Background(), &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "write-smoke", Namespace: "default"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if method != http.MethodPost || path != "/clusters/kind-realmroot/kubernetes/api/v1/namespaces/default/configmaps" {
		t.Fatalf("request = %s %s", method, path)
	}
	if authorization != "Bearer request-id-token" {
		t.Fatalf("Authorization = %q", authorization)
	}
}

func TestInventoryCatalogReplacesAndDeletesContexts(t *testing.T) {
	catalog := &inventoryCatalog{contexts: make(map[string]*inventoryContext)}
	profile := inventoryProfile("https://one.example.test")
	catalog.upsert(profile)
	profile.Status.AccessProviders[0].Cluster.Server = "https://two.example.test"
	catalog.upsert(profile)
	items := catalog.list()
	if len(items) != 1 || items[0].config.Host != "https://two.example.test" {
		t.Fatalf("contexts = %#v", items)
	}
	catalog.remove(profile)
	if len(catalog.list()) != 0 {
		t.Fatal("deleted ClusterProfile remained in the catalog")
	}
}

func inventoryProfile(server string) *apisv1alpha1.ClusterProfile {
	return &apisv1alpha1.ClusterProfile{
		ObjectMeta: metav1.ObjectMeta{Name: "kind-realmroot", Namespace: "cluster-inventory"},
		Spec: apisv1alpha1.ClusterProfileSpec{
			DisplayName:    "Kind Realmroot",
			ClusterManager: apisv1alpha1.ClusterManager{Name: "kube-cluster-hub"},
		},
		Status: apisv1alpha1.ClusterProfileStatus{
			AccessProviders: []apisv1alpha1.AccessProvider{{
				Name:    "current-user-oidc",
				Cluster: clientcmdv1.Cluster{Server: server},
			}},
		},
	}
}
