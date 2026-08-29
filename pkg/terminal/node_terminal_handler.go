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
	"k8s.io/klog/v2"
)

type NodeTerminalHandler struct{}

func NewNodeTerminalHandler() *NodeTerminalHandler {
	return &NodeTerminalHandler{}
}

func (h *NodeTerminalHandler) HandleNodeTerminalWebSocket(c *gin.Context) {
	cs := c.MustGet("cluster").(*cluster.ClientSet)
	nodeName := strings.TrimSpace(c.Param("nodeName"))
	if nodeName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "node name is required"})
		return
	}
	if cs.K8sClient.Configuration == nil || strings.TrimSpace(cs.K8sClient.Configuration.BearerToken) == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "the current OIDC session has no Kubernetes bearer token"})
		return
	}
	setting, err := model.GetGeneralSetting()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load node terminal settings"})
		return
	}
	image := strings.TrimSpace(setting.NodeTerminalImage)
	if image == "" {
		image = common.NodeTerminalImage
	}

	wsutil.Serve(c.Writer, c.Request, func(ws *wsutil.Session) {
		if _, err := cs.K8sClient.ClientSet.CoreV1().Nodes().Get(ws.Context, nodeName, metav1.GetOptions{}); err != nil {
			ws.SendErrorMessage(fmt.Sprintf("failed to get node %s: %v", nodeName, err))
			return
		}

		pod := buildNodeTerminalPod(nodeName, image)
		if err := cs.K8sClient.Create(ws.Context, pod); err != nil {
			ws.SendErrorMessage(fmt.Sprintf("failed to create node terminal pod: %v", err))
			return
		}
		defer h.cleanupPod(cs, pod.Name)

		if err := waitForAgentPodReady(ws.Context, cs, ws, pod.Name, "node terminal ready"); err != nil {
			klog.V(2).Infof("node terminal pod %s did not become ready: %v", pod.Name, err)
			return
		}

		session := kube.NewTerminalSession(cs.K8sClient, ws.Conn, common.AgentPodNamespace, pod.Name, common.NodeTerminalPodName)
		defer session.Close()
		command := []string{"sh", "-c", "exec chroot /host /bin/sh -l || exec chroot /host /bin/bash || exec /bin/sh"}
		if err := session.StartCommand(ws.Context, "exec", command); err != nil {
			klog.V(2).Infof("node terminal session %s ended: %v", pod.Name, err)
		}
	})
}

func buildNodeTerminalPod(nodeName, image string) *corev1.Pod {
	privileged := true
	automount := false
	readOnly := false
	gracePeriod := int64(0)
	activeDeadline := int64(60 * 60)
	hostPathType := corev1.HostPathDirectory
	name := utils.GenerateNodeAgentName(nodeName)
	labels := map[string]string{
		"app.kubernetes.io/managed-by": "lightkite",
		"lightkite.io/component":       "node-terminal",
		"lightkite.io/session":         name,
	}
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: common.AgentPodNamespace, Labels: labels},
		Spec: corev1.PodSpec{
			NodeName:                      nodeName,
			AutomountServiceAccountToken:  &automount,
			RestartPolicy:                 corev1.RestartPolicyNever,
			ActiveDeadlineSeconds:         &activeDeadline,
			TerminationGracePeriodSeconds: &gracePeriod,
			Tolerations:                   []corev1.Toleration{{Operator: corev1.TolerationOpExists}},
			Volumes: []corev1.Volume{{
				Name: "host",
				VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{
					Path: "/",
					Type: &hostPathType,
				}},
			}},
			Containers: []corev1.Container{{
				Name:            common.NodeTerminalPodName,
				Image:           image,
				ImagePullPolicy: corev1.PullIfNotPresent,
				Command:         []string{"sh", "-c", "trap : TERM INT; sleep 86400 & wait"},
				SecurityContext: &corev1.SecurityContext{Privileged: &privileged},
				VolumeMounts:    []corev1.VolumeMount{{Name: "host", MountPath: "/host", ReadOnly: readOnly}},
			}},
		},
	}
}

func (h *NodeTerminalHandler) cleanupPod(cs *cluster.ClientSet, name string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	gracePeriod := int64(0)
	err := cs.K8sClient.ClientSet.CoreV1().Pods(common.AgentPodNamespace).Delete(ctx, name, metav1.DeleteOptions{GracePeriodSeconds: &gracePeriod})
	if err != nil && !apierrors.IsNotFound(err) {
		klog.Warningf("failed to clean up node terminal pod %s/%s: %v", common.AgentPodNamespace, name, err)
	}
}
