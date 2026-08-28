package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/zxh326/kite/pkg/cluster"
	"github.com/zxh326/kite/pkg/helmutil"
	"github.com/zxh326/kite/pkg/model"
	"gorm.io/gorm"
	release "helm.sh/helm/v4/pkg/release/v1"
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
	cm     *cluster.ClusterManager
	tokens IdentityTokenProvider
}

type IdentityTokenProvider interface {
	IDTokenForSession(context.Context, uint) (string, error)
}

func registerHelmReleaseAutoUpgradeExecutor(manager *Manager, cm *cluster.ClusterManager, tokens IdentityTokenProvider) {
	manager.Register(HelmReleaseAutoUpgradeTaskType, &helmReleaseAutoUpgradeExecutor{cm: cm, tokens: tokens})
}

func (e *helmReleaseAutoUpgradeExecutor) Run(ctx context.Context, task model.ScheduledTask) error {
	var payload HelmReleaseAutoUpgradePayload
	if err := json.Unmarshal([]byte(task.Payload), &payload); err != nil {
		return err
	}
	if task.CreatorID == 0 {
		return fmt.Errorf("scheduled task creator is missing")
	}
	if task.OIDCSessionID == 0 {
		return &permanentTaskError{err: fmt.Errorf("scheduled task has no authorized OIDC session; re-enable it to authorize the task")}
	}
	session, err := model.GetOIDCSessionByID(model.DB.WithContext(ctx), task.OIDCSessionID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return &permanentTaskError{err: fmt.Errorf("scheduled task OIDC session no longer exists; re-enable it to authorize the task")}
		}
		return err
	}
	if session.UserID != task.CreatorID {
		return &permanentTaskError{err: fmt.Errorf("scheduled task identity no longer matches its authorized OIDC session; re-enable it to authorize the task")}
	}
	idToken, err := e.tokens.IDTokenForSession(ctx, task.OIDCSessionID)
	if err != nil {
		return err
	}
	session, err = model.GetOIDCSessionByID(model.DB.WithContext(ctx), task.OIDCSessionID)
	if err != nil {
		return err
	}
	cs, err := e.cm.GetClientSet(task.ClusterName, idToken, string(session.AccessToken))
	if err != nil {
		return err
	}
	cfg, err := helmutil.NewActionConfig(cs.K8sClient.Configuration, helmutil.StorageNamespace(payload.Namespace))
	if err != nil {
		return err
	}
	current, err := helmutil.GetRelease(cfg, payload.ResourceName)
	if err != nil {
		return err
	}
	if current.Chart == nil {
		return fmt.Errorf("helm release chart is missing")
	}

	_, currentVersion, _ := helmutil.ChartInfo(current)
	nextChart, err := helmutil.LatestChartPackage(ctx, payload.Source, payload.RepositoryName, payload.ChartName)
	if err != nil {
		return err
	}
	if !helmutil.IsChartVersionNewer(nextChart.Version, currentVersion) {
		return nil
	}
	loadedChart, err := helmutil.LoadArchiveContext(ctx, nextChart.URL, nextChart.Repository)
	if err != nil {
		return err
	}

	var next *release.Release
	var runErr error
	success := false
	defer func() {
		helmutil.RecordReleaseHistory(cs.ClusterID, cs.Name, task.CreatorID, "auto", "upgrade", payload.ResourceName, payload.Namespace, current, next, success, runErr)
	}()
	next, err = helmutil.UpgradeRelease(ctx, cfg, payload.ResourceName, loadedChart, map[string]interface{}{}, helmutil.UpgradeReleaseOptions{
		Namespace:            payload.Namespace,
		Timeout:              time.Duration(payload.TimeoutMinutes) * time.Minute,
		ResetThenReuseValues: true,
		Description:          "Automated upgrade requested from Kite",
		RollbackOnFailure:    payload.RollbackOnFailure,
	})
	if err != nil {
		runErr = err
		return err
	}
	success = true
	upgradedAt := time.Now()
	payload.LastUpgradedAt = &upgradedAt
	return saveHelmAutoUpgradePayload(task.ID, payload)
}

type permanentTaskError struct{ err error }

func (e *permanentTaskError) Error() string     { return e.err.Error() }
func (e *permanentTaskError) Unwrap() error     { return e.err }
func (e *permanentTaskError) IsPermanent() bool { return true }

func saveHelmAutoUpgradePayload(taskID uint, payload HelmReleaseAutoUpgradePayload) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return model.DB.Model(&model.ScheduledTask{}).Where("id = ?", taskID).Update("payload", string(data)).Error
}

func HelmReleaseAutoUpgradeTaskKey(namespace, releaseName string) string {
	return namespace + "/" + releaseName
}

func HelmReleaseAutoUpgradeTaskName(namespace, releaseName string) string {
	return fmt.Sprintf("Helm release auto upgrade %s/%s", namespace, releaseName)
}
