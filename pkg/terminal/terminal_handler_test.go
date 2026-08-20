package terminal

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/zxh326/kite/pkg/cluster"
	"github.com/zxh326/kite/pkg/common"
	"github.com/zxh326/kite/pkg/kube"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

func TestPodTerminalValidatesTargetBeforeOpeningWebSocket(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/terminal", nil)
	c.Set("cluster", &cluster.ClientSet{Name: "prod", K8sClient: &kube.K8sClient{}})

	(&TerminalHandler{}).HandleTerminalWebSocket(c)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", recorder.Code)
	}
}

func TestBuildKubectlSessionResourcesUsesOnlyCurrentUserIdentity(t *testing.T) {
	clientSet := &cluster.ClientSet{
		Name: "prod",
		K8sClient: &kube.K8sClient{Configuration: &rest.Config{
			BearerToken: "current-user-id-token",
			TLSClientConfig: rest.TLSClientConfig{
				CAData: []byte("test-ca"),
			},
		}},
	}

	secret, pod, err := buildKubectlSessionResources(clientSet, "kite-kubectl-test", "kubectl:test")
	if err != nil {
		t.Fatalf("build resources: %v", err)
	}
	config, err := clientcmd.Load(secret.Data["config"])
	if err != nil {
		t.Fatalf("load generated kubeconfig: %v", err)
	}
	if got := config.AuthInfos["oidc-user"].Token; got != "current-user-id-token" {
		t.Fatalf("kubeconfig token = %q", got)
	}
	if got := config.Clusters["target"].Server; got != "https://kubernetes.default.svc" {
		t.Fatalf("kubeconfig server = %q", got)
	}
	if !bytes.Equal(config.Clusters["target"].CertificateAuthorityData, []byte("test-ca")) {
		t.Fatalf("kubeconfig CA = %q", config.Clusters["target"].CertificateAuthorityData)
	}
	if pod.Spec.AutomountServiceAccountToken == nil || *pod.Spec.AutomountServiceAccountToken {
		t.Fatal("kubectl session pod must not mount a ServiceAccount token")
	}
	if pod.Spec.ServiceAccountName != "" {
		t.Fatalf("service account = %q, want empty", pod.Spec.ServiceAccountName)
	}
	container := pod.Spec.Containers[0]
	if container.SecurityContext == nil || container.SecurityContext.AllowPrivilegeEscalation == nil || *container.SecurityContext.AllowPrivilegeEscalation {
		t.Fatal("kubectl session container permits privilege escalation")
	}
	if container.SecurityContext.Privileged != nil && *container.SecurityContext.Privileged {
		t.Fatal("kubectl session container is privileged")
	}
	if secret.Namespace != common.AgentPodNamespace || pod.Namespace != common.AgentPodNamespace {
		t.Fatalf("session namespaces = %q/%q", secret.Namespace, pod.Namespace)
	}
}

func TestBuildKubectlSessionResourcesRejectsMissingUserToken(t *testing.T) {
	clientSet := &cluster.ClientSet{K8sClient: &kube.K8sClient{Configuration: &rest.Config{}}}
	if _, _, err := buildKubectlSessionResources(clientSet, "test", "kubectl:test"); err == nil {
		t.Fatal("expected missing token error")
	}
}

func TestBuildNodeTerminalPodUsesKubernetesAuthorizedEphemeralSession(t *testing.T) {
	pod := buildNodeTerminalPod("worker-a", "node-shell:test")

	if pod.Spec.NodeName != "worker-a" || pod.Spec.Containers[0].Image != "node-shell:test" {
		t.Fatalf("unexpected node terminal target: node=%q image=%q", pod.Spec.NodeName, pod.Spec.Containers[0].Image)
	}
	if pod.Spec.AutomountServiceAccountToken == nil || *pod.Spec.AutomountServiceAccountToken {
		t.Fatal("node terminal pod must not mount a ServiceAccount token")
	}
	if pod.Spec.ServiceAccountName != "" {
		t.Fatalf("service account = %q, want empty", pod.Spec.ServiceAccountName)
	}
	if pod.Spec.ActiveDeadlineSeconds == nil || *pod.Spec.ActiveDeadlineSeconds != 3600 {
		t.Fatalf("active deadline = %v, want 3600", pod.Spec.ActiveDeadlineSeconds)
	}
	container := pod.Spec.Containers[0]
	if container.SecurityContext == nil || container.SecurityContext.Privileged == nil || !*container.SecurityContext.Privileged {
		t.Fatal("node terminal must explicitly declare its privileged host-access boundary")
	}
	if len(pod.Spec.Volumes) != 1 || pod.Spec.Volumes[0].HostPath == nil || pod.Spec.Volumes[0].HostPath.Path != "/" {
		t.Fatal("node terminal must mount the selected node root")
	}
	if pod.Spec.HostNetwork || pod.Spec.HostPID || pod.Spec.HostIPC {
		t.Fatal("node terminal does not need host network, PID, or IPC namespaces")
	}
}
