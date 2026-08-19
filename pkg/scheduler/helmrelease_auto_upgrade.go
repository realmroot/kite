package scheduler

import (
	"context"
	"fmt"
	"time"

	"github.com/zxh326/kite/pkg/cluster"
	"github.com/zxh326/kite/pkg/model"
)

const (
	HelmReleaseAutoUpgradeTaskType = "helm_release_auto_upgrade"
)

type HelmReleaseAutoUpgradePayload struct {
	Namespace         string     `json:"namespace"`
	ResourceType      string     `json:"resourceType"`
	ResourceName      string     `json:"resourceName"`
	Source            string     `json:"source"`
	RepositoryName    string     `json:"repositoryName"`
	ChartName         string     `json:"chartName"`
	TimeoutMinutes    int        `json:"timeoutMinutes"`
	RollbackOnFailure bool       `json:"rollbackOnFailure"`
	LastUpgradedAt    *time.Time `json:"lastUpgradedAt,omitempty"`
}

type helmReleaseAutoUpgradeExecutor struct {
	cm *cluster.ClusterManager
}

func registerHelmReleaseAutoUpgradeExecutor(manager *Manager, cm *cluster.ClusterManager) {
	manager.Register(HelmReleaseAutoUpgradeTaskType, &helmReleaseAutoUpgradeExecutor{cm: cm})
}

func (e *helmReleaseAutoUpgradeExecutor) Run(ctx context.Context, task model.ScheduledTask) error {
	return fmt.Errorf("scheduled Kubernetes operations are disabled because no interactive OIDC identity is available")
}

func HelmReleaseAutoUpgradeTaskKey(namespace, releaseName string) string {
	return namespace + "/" + releaseName
}

func HelmReleaseAutoUpgradeTaskName(namespace, releaseName string) string {
	return fmt.Sprintf("Helm release auto upgrade %s/%s", namespace, releaseName)
}
