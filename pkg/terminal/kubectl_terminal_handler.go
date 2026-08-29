package terminal

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/realmroot/lightkite/pkg/cluster"
	"github.com/realmroot/lightkite/pkg/common"
	"github.com/realmroot/lightkite/pkg/kube"
	"github.com/realmroot/lightkite/pkg/model"
	"github.com/realmroot/lightkite/pkg/utils"
	"github.com/realmroot/lightkite/pkg/wsutil"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const kubectlConfigMountPath = "/home/kubectl/.kube"

type KubectlTerminalHandler struct{}

func NewKubectlTerminalHandler() *KubectlTerminalHandler {
	return &KubectlTerminalHandler{}
}

func (h *KubectlTerminalHandler) HandleKubectlTerminalWebSocket(c *gin.Context) {
	cs := c.MustGet("cluster").(*cluster.ClientSet)
	user := c.MustGet("user").(model.User)
	setting, err := model.GetGeneralSetting()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load kubectl terminal settings"})
		return
	}
	if !setting.KubectlEnabled {
		c.JSON(http.StatusNotFound, gin.H{"error": "kubectl terminal is disabled"})
		return
	}
	if cs.K8sClient.Configuration == nil || strings.TrimSpace(cs.K8sClient.Configuration.BearerToken) == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "the current OIDC session has no Kubernetes bearer token"})
		return
	}
	kubectlImage := strings.TrimSpace(setting.KubectlImage)
	if kubectlImage == "" {
		kubectlImage = common.KubectlTerminalImage
	}

	wsutil.Serve(c.Writer, c.Request, func(ws *wsutil.Session) {
		instanceID := utils.GenerateKubectlAgentName(user.PrincipalKey())
		secret, pod, err := buildKubectlSessionResources(cs, instanceID, kubectlImage)
		if err != nil {
			ws.SendErrorMessage(err.Error())
			return
		}
		if err := cs.K8sClient.Create(ws.Context, pod); err != nil {
			ws.SendErrorMessage(fmt.Sprintf("failed to create kubectl session pod: %v", err))
			return
		}
		defer h.cleanupSession(cs, instanceID)
		controller := true
		blockOwnerDeletion := false
		secret.OwnerReferences = []metav1.OwnerReference{{
			APIVersion:         "v1",
			Kind:               "Pod",
			Name:               pod.Name,
			UID:                pod.UID,
			Controller:         &controller,
			BlockOwnerDeletion: &blockOwnerDeletion,
		}}
		if err := cs.K8sClient.Create(ws.Context, secret); err != nil {
			ws.SendErrorMessage(fmt.Sprintf("failed to create kubectl session credential: %v", err))
			return
		}
		if err := waitForAgentPodReady(ws.Context, cs, ws, pod.Name, "kubectl session ready"); err != nil {
			klog.V(2).Infof("kubectl session pod %s did not become ready: %v", pod.Name, err)
			return
		}

		session := kube.NewTerminalSession(cs.K8sClient, ws.Conn, common.AgentPodNamespace, pod.Name, common.KubectlTerminalPodName)
		defer session.Close()
		if err := session.Start(ws.Context, "exec"); err != nil {
			klog.V(2).Infof("kubectl terminal session %s ended: %v", instanceID, err)
		}
	})
}

func buildKubectlSessionResources(cs *cluster.ClientSet, instanceID, image string) (*corev1.Secret, *corev1.Pod, error) {
	if cs.K8sClient.Configuration == nil || strings.TrimSpace(cs.K8sClient.Configuration.BearerToken) == "" {
		return nil, nil, fmt.Errorf("kubernetes bearer token is required")
	}
	kubeconfig, err := clientcmd.Write(clientcmdapi.Config{
		Clusters: map[string]*clientcmdapi.Cluster{
			"target": {
				Server:                   "https://kubernetes.default.svc",
				CertificateAuthorityData: append([]byte(nil), cs.K8sClient.Configuration.CAData...),
				InsecureSkipTLSVerify:    cs.K8sClient.Configuration.Insecure,
				TLSServerName:            "kubernetes.default.svc",
			},
		},
		AuthInfos: map[string]*clientcmdapi.AuthInfo{
			"oidc-user": {Token: cs.K8sClient.Configuration.BearerToken},
		},
		Contexts: map[string]*clientcmdapi.Context{
			"target": {Cluster: "target", AuthInfo: "oidc-user"},
		},
		CurrentContext: "target",
	})
	if err != nil {
		return nil, nil, fmt.Errorf("build kubectl session configuration: %w", err)
	}

	labels := map[string]string{
		"app.kubernetes.io/managed-by": "lightkite",
		"lightkite.io/component":       "kubectl-terminal",
		"lightkite.io/session":         instanceID,
	}
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: instanceID, Namespace: common.AgentPodNamespace, Labels: labels},
		Type:       corev1.SecretTypeOpaque,
		Data:       map[string][]byte{"config": kubeconfig},
	}
	automount := false
	readOnly := true
	nonRoot := true
	runAsUser := int64(65532)
	defaultMode := int32(0440)
	gracePeriod := int64(0)
	activeDeadline := int64(60 * 60)
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: instanceID, Namespace: common.AgentPodNamespace, Labels: labels},
		Spec: corev1.PodSpec{
			AutomountServiceAccountToken:  &automount,
			RestartPolicy:                 corev1.RestartPolicyNever,
			ActiveDeadlineSeconds:         &activeDeadline,
			TerminationGracePeriodSeconds: &gracePeriod,
			SecurityContext: &corev1.PodSecurityContext{
				RunAsNonRoot: &nonRoot,
				RunAsUser:    &runAsUser,
				RunAsGroup:   &runAsUser,
				FSGroup:      &runAsUser,
			},
			Volumes: []corev1.Volume{{
				Name: "kubeconfig",
				VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{
					SecretName:  secret.Name,
					DefaultMode: &defaultMode,
				}},
			}},
			Containers: []corev1.Container{{
				Name:            common.KubectlTerminalPodName,
				Image:           image,
				ImagePullPolicy: corev1.PullIfNotPresent,
				Command:         []string{"sh", "-c", "trap : TERM INT; sleep infinity & wait"},
				Env: []corev1.EnvVar{{
					Name:  "KUBECONFIG",
					Value: kubectlConfigMountPath + "/config",
				}},
				Stdin: true,
				TTY:   true,
				SecurityContext: &corev1.SecurityContext{
					AllowPrivilegeEscalation: &automount,
					RunAsNonRoot:             &nonRoot,
					RunAsUser:                &runAsUser,
					Capabilities:             &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}},
					SeccompProfile:           &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault},
				},
				VolumeMounts: []corev1.VolumeMount{{Name: "kubeconfig", MountPath: kubectlConfigMountPath, ReadOnly: readOnly}},
			}},
		},
	}
	return secret, pod, nil
}

func (h *KubectlTerminalHandler) cleanupSession(cs *cluster.ClientSet, instanceID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	gracePeriod := int64(0)
	propagation := metav1.DeletePropagationBackground
	resources := []client.Object{
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: instanceID, Namespace: common.AgentPodNamespace}},
		&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: instanceID, Namespace: common.AgentPodNamespace}},
	}
	for _, resource := range resources {
		if err := cs.K8sClient.Delete(ctx, resource, &client.DeleteOptions{
			GracePeriodSeconds: &gracePeriod,
			PropagationPolicy:  &propagation,
		}); err != nil && !apierrors.IsNotFound(err) {
			klog.Warningf("failed to clean up kubectl session resource %s/%s: %v", resource.GetNamespace(), resource.GetName(), err)
		}
	}
}
