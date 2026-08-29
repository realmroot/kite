package resources

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/realmroot/lightkite/pkg/cluster"
	"github.com/realmroot/lightkite/pkg/common"
	"github.com/realmroot/lightkite/pkg/helmutil"
	"github.com/realmroot/lightkite/pkg/model"
	"helm.sh/helm/v4/pkg/action"
	release "helm.sh/helm/v4/pkg/release/v1"
	"helm.sh/helm/v4/pkg/storage/driver"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/klog/v2"
)

const (
	helmActionTimeout = 5 * time.Minute
)

type HelmReleaseHandler struct{}

type helmReleaseRunResult struct {
	current *release.Release
	release *release.Release
}

type helmReleaseInstallRequest struct {
	ReleaseName     string                 `json:"releaseName" binding:"required"`
	Namespace       string                 `json:"namespace"`
	RepositoryName  string                 `json:"repositoryName"`
	Source          string                 `json:"source"`
	ChartName       string                 `json:"chartName" binding:"required"`
	ChartVersion    string                 `json:"chartVersion"`
	Values          map[string]interface{} `json:"values"`
	Description     string                 `json:"description"`
	CreateNamespace bool                   `json:"createNamespace"`
	Wait            bool                   `json:"wait"`
}

func NewHelmReleaseHandler() *HelmReleaseHandler    { return &HelmReleaseHandler{} }
func (h *HelmReleaseHandler) IsClusterScoped() bool { return false }
func (h *HelmReleaseHandler) Searchable() bool      { return false }
func (h *HelmReleaseHandler) ListHistory(c *gin.Context) {
	cfg, err := h.actionConfig(c, c.Param("namespace"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	items, err := helmutil.ReleaseHistoryItems(cfg, c.Param("name"))
	if err != nil {
		writeHelmError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}
func (h *HelmReleaseHandler) Create(c *gin.Context) {
	rel, status, err := h.runInstall(c, false)
	if err != nil {
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}

	result := helmutil.ToHelmRelease(rel, true)
	c.JSON(http.StatusCreated, result)
}

func (h *HelmReleaseHandler) DryRunInstall(c *gin.Context) {
	rel, status, err := h.runInstall(c, true)
	if err != nil {
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, helmutil.ToHelmReleaseDryRunResponse(rel))
}

func (h *HelmReleaseHandler) runInstall(c *gin.Context, dryRun bool) (rel *release.Release, status int, err error) {
	ctx := c.Request.Context()
	namespace := strings.TrimSpace(c.Param("namespace"))
	if namespace == "" || namespace == common.AllNamespaces {
		return nil, http.StatusBadRequest, fmt.Errorf("namespace is required")
	}

	var req helmReleaseInstallRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		return nil, http.StatusBadRequest, err
	}
	req.ReleaseName = strings.TrimSpace(req.ReleaseName)
	req.Namespace = strings.TrimSpace(req.Namespace)
	req.RepositoryName = strings.TrimSpace(req.RepositoryName)
	req.Source = strings.TrimSpace(req.Source)
	req.ChartName = strings.TrimSpace(req.ChartName)
	req.ChartVersion = strings.TrimSpace(req.ChartVersion)
	if req.ReleaseName == "" {
		return nil, http.StatusBadRequest, fmt.Errorf("releaseName is required")
	}
	if req.Namespace != "" && req.Namespace != namespace {
		return nil, http.StatusBadRequest, fmt.Errorf("request namespace does not match URL namespace")
	}
	if !dryRun {
		defer func() {
			h.recordHistory(c, "install", req.ReleaseName, namespace, nil, rel, err == nil, err)
		}()
	}

	chartPackage, err := helmutil.ResolveChartPackage(ctx, req.Source, req.RepositoryName, req.ChartName, req.ChartVersion)
	if err != nil {
		return nil, http.StatusBadRequest, fmt.Errorf("resolve chart package: %w", err)
	}
	loadedChart, err := helmutil.LoadArchiveContext(ctx, chartPackage.URL, chartPackage.Repository)
	if err != nil {
		return nil, http.StatusBadRequest, err
	}

	cfg, err := h.actionConfig(c, namespace)
	if err != nil {
		return nil, helmErrorStatus(err), err
	}
	values := req.Values
	if values == nil {
		values = map[string]interface{}{}
	}
	description := req.Description
	if description == "" {
		description = "Install requested from Lightkite"
		if dryRun {
			description = "Dry run install requested from Lightkite"
		}
	}

	rel, err = helmutil.InstallRelease(ctx, cfg, loadedChart, values, helmutil.InstallReleaseOptions{
		ReleaseName:     req.ReleaseName,
		Namespace:       namespace,
		Timeout:         helmActionTimeout,
		Description:     description,
		CreateNamespace: req.CreateNamespace,
		DryRun:          dryRun,
		Wait:            req.Wait,
	})
	if err != nil {
		return nil, helmErrorStatus(err), err
	}
	return rel, http.StatusOK, nil
}

func (h *HelmReleaseHandler) Describe(c *gin.Context) {
	obj, err := h.get(c, c.Param("namespace"), c.Param("name"), true)
	if err != nil {
		writeHelmError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"result": fmt.Sprintf(
			"Name: %s\nNamespace: %s\nRevision: %d\nStatus: %s\nChart: %s\nDescription: %s\n",
			obj.Name,
			obj.Namespace,
			obj.Spec.Revision,
			obj.Status.Status,
			obj.Spec.Chart,
			obj.Spec.Description,
		),
	})
}

func (h *HelmReleaseHandler) registerCustomRoutes(group *gin.RouterGroup) {
	group.POST("/:namespace/dry-run", h.DryRunInstall)
	group.PUT("/:namespace/:name/upgrade", h.Upgrade)
	group.PUT("/:namespace/:name/upgrade/dry-run", h.DryRunUpgrade)
	group.PUT("/:namespace/:name/rollback", h.Rollback)
}

func (h *HelmReleaseHandler) List(c *gin.Context) {
	labelSelector := c.Query("labelSelector")
	if _, err := labels.Parse(labelSelector); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid labelSelector parameter: " + err.Error()})
		return
	}
	list, err := h.list(c, c.Param("namespace"), false)
	if err != nil {
		writeHelmError(c, err)
		return
	}
	c.JSON(http.StatusOK, list)
}
func (h *HelmReleaseHandler) Get(c *gin.Context) {
	obj, err := h.get(c, c.Param("namespace"), c.Param("name"), true)
	if err != nil {
		writeHelmError(c, err)
		return
	}
	c.JSON(http.StatusOK, obj)
}
func (h *HelmReleaseHandler) GetResource(c *gin.Context, namespace, name string) (interface{}, error) {
	return h.get(c, namespace, name, true)
}

func (h *HelmReleaseHandler) Search(c *gin.Context, q string, limit int64) ([]common.SearchResult, error) {
	list, err := h.list(c, common.AllNamespaces, false)
	if err != nil {
		return nil, err
	}
	results := []common.SearchResult{}
	for _, item := range list.Items {
		if !strings.Contains(strings.ToLower(item.Name), strings.ToLower(q)) {
			continue
		}
		results = append(results, common.SearchResult{
			ID:           helmReleaseID(item),
			Name:         item.Name,
			Namespace:    item.Namespace,
			ResourceType: string(common.HelmReleases),
			CreatedAt:    item.CreationTimestamp.String(),
		})
		if limit > 0 && int64(len(results)) >= limit {
			break
		}
	}
	return results, nil
}

func (h *HelmReleaseHandler) Delete(c *gin.Context) {
	cfg, err := h.actionConfig(c, c.Param("namespace"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	current, err := helmutil.GetRelease(cfg, c.Param("name"))
	if err != nil {
		writeHelmError(c, err)
		return
	}
	success := false
	var runErr error
	defer func() {
		h.recordHistory(c, "delete", c.Param("name"), c.Param("namespace"), current, nil, success, runErr)
	}()

	if err := helmutil.UninstallRelease(cfg, c.Param("name"), helmutil.UninstallReleaseOptions{
		Timeout:     helmActionTimeout,
		Description: "Deleted from Lightkite",
	}); err != nil {
		runErr = err
		writeHelmError(c, err)
		return
	}
	success = true
	c.JSON(http.StatusOK, gin.H{"message": "helm release deleted"})
}

type helmReleaseActionRequest struct {
	Revision          int                    `json:"revision"`
	RepositoryName    string                 `json:"repositoryName"`
	Source            string                 `json:"source"`
	ChartName         string                 `json:"chartName"`
	ChartVersion      string                 `json:"chartVersion"`
	Values            map[string]interface{} `json:"values"`
	Description       string                 `json:"description"`
	ForceConflicts    bool                   `json:"forceConflicts"`
	Wait              bool                   `json:"wait"`
	RollbackOnFailure bool                   `json:"rollbackOnFailure"`
}

func (h *HelmReleaseHandler) Upgrade(c *gin.Context) {
	_, status, err := h.runUpgrade(c, false)
	if err != nil {
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "helm release upgraded"})
}

func (h *HelmReleaseHandler) DryRunUpgrade(c *gin.Context) {
	result, status, err := h.runUpgrade(c, true)
	if err != nil {
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, helmutil.ToHelmReleaseDryRunDiffResponse(result.current, result.release))
}

func (h *HelmReleaseHandler) runUpgrade(c *gin.Context, dryRun bool) (result helmReleaseRunResult, status int, err error) {
	ctx := c.Request.Context()
	namespace, name := c.Param("namespace"), c.Param("name")
	var req helmReleaseActionRequest
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		return helmReleaseRunResult{}, http.StatusBadRequest, err
	}
	if strings.TrimSpace(req.ChartName) == "" &&
		(strings.TrimSpace(req.RepositoryName) != "" || strings.TrimSpace(req.Source) != "" || strings.TrimSpace(req.ChartVersion) != "") {
		return helmReleaseRunResult{}, http.StatusBadRequest, fmt.Errorf("chartName is required when selecting a different chart package")
	}

	cfg, err := h.actionConfig(c, namespace)
	if err != nil {
		return helmReleaseRunResult{}, http.StatusInternalServerError, err
	}
	current, err := helmutil.GetRelease(cfg, name)
	if err != nil {
		return helmReleaseRunResult{}, helmErrorStatus(err), err
	}
	if current.Chart == nil {
		return helmReleaseRunResult{}, http.StatusInternalServerError, fmt.Errorf("helm release chart is missing")
	}
	result.current = current
	if !dryRun {
		defer func() {
			h.recordHistory(c, "upgrade", name, namespace, current, result.release, err == nil, err)
		}()
	}

	chartToUpgrade := current.Chart
	if strings.TrimSpace(req.ChartName) != "" {
		chartPackage, err := helmutil.ResolveChartPackage(
			ctx,
			strings.TrimSpace(req.Source),
			strings.TrimSpace(req.RepositoryName),
			strings.TrimSpace(req.ChartName),
			strings.TrimSpace(req.ChartVersion),
		)
		if err != nil {
			return helmReleaseRunResult{}, http.StatusBadRequest, fmt.Errorf("resolve chart package: %w", err)
		}
		chartToUpgrade, err = helmutil.LoadArchiveContext(ctx, chartPackage.URL, chartPackage.Repository)
		if err != nil {
			return helmReleaseRunResult{}, http.StatusBadRequest, err
		}
	}

	values := req.Values
	if values == nil {
		values = map[string]interface{}{}
	}
	description := req.Description
	if description == "" {
		description = "Dry run upgrade requested from Lightkite"
		if !dryRun {
			description = "Upgrade requested from Lightkite"
		}
	}

	rel, err := helmutil.UpgradeRelease(ctx, cfg, name, chartToUpgrade, values, helmutil.UpgradeReleaseOptions{
		Namespace:         namespace,
		Timeout:           helmActionTimeout,
		ReuseValues:       req.Values == nil,
		Description:       description,
		ForceConflicts:    req.ForceConflicts,
		RollbackOnFailure: req.RollbackOnFailure,
		DryRun:            dryRun,
		Wait:              req.Wait,
	})
	if err != nil {
		return helmReleaseRunResult{}, helmErrorStatus(err), err
	}
	result.release = rel
	return result, http.StatusOK, nil
}

func (h *HelmReleaseHandler) Rollback(c *gin.Context) {
	namespace, name := c.Param("namespace"), c.Param("name")
	var req helmReleaseActionRequest
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	cfg, err := h.actionConfig(c, namespace)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	current, err := helmutil.GetRelease(cfg, name)
	if err != nil {
		writeHelmError(c, err)
		return
	}
	success := false
	var next *release.Release
	var runErr error
	defer func() {
		h.recordHistory(c, "rollback", name, namespace, current, next, success, runErr)
	}()

	targetRevision := req.Revision
	if targetRevision == 0 {
		targetRevision = current.Version - 1
	}
	if targetRevision <= 0 {
		runErr = fmt.Errorf("no previous helm release revision found")
		c.JSON(http.StatusBadRequest, gin.H{"error": "no previous helm release revision found"})
		return
	}

	if err := helmutil.RollbackRelease(cfg, name, helmutil.RollbackReleaseOptions{
		Version: targetRevision,
		Timeout: helmActionTimeout,
	}); err != nil {
		runErr = err
		writeHelmError(c, err)
		return
	}
	if next, err = helmutil.GetRelease(cfg, name); err != nil {
		klog.Errorf("Failed to read rolled back helm release: %v", err)
	}
	success = true
	c.JSON(http.StatusOK, gin.H{"message": "helm release rolled back", "revision": targetRevision})
}

func (h *HelmReleaseHandler) recordHistory(c *gin.Context, opType, name, namespace string, prev, curr *release.Release, success bool, err error) {
	cs := c.MustGet("cluster").(*cluster.ClientSet)
	user := c.MustGet("user").(model.User)
	helmutil.RecordReleaseHistory(cs.ClusterID, cs.Name, user.ID, "manual", opType, name, namespace, prev, curr, success, err)
}

func helmErrorStatus(err error) int {
	if errors.Is(err, driver.ErrReleaseNotFound) {
		return http.StatusNotFound
	}
	return kubernetesStatusCode(err, http.StatusInternalServerError)
}

func writeHelmError(c *gin.Context, err error) {
	c.JSON(helmErrorStatus(err), gin.H{"error": err.Error()})
}

func (h *HelmReleaseHandler) list(c *gin.Context, namespace string, details bool) (*helmutil.HelmReleaseList, error) {
	cs := c.MustGet("cluster").(*cluster.ClientSet)
	allNamespaces := namespace == "" || namespace == common.AllNamespaces
	cfg, err := h.actionConfigForClientSet(cs, helmutil.StorageNamespace(namespace))
	if err != nil {
		return nil, err
	}
	releases, err := helmutil.ListReleases(cfg, allNamespaces, c.Query("labelSelector"))
	if err != nil {
		return nil, err
	}

	items := make([]helmutil.HelmRelease, 0, len(releases))
	for _, rel := range releases {
		items = append(items, helmutil.ToHelmRelease(rel, details))
	}
	return &helmutil.HelmReleaseList{TypeMeta: metav1.TypeMeta{Kind: "HelmReleaseList", APIVersion: "v1"}, Items: items}, nil
}

func (h *HelmReleaseHandler) get(c *gin.Context, namespace, name string, details bool) (*helmutil.HelmRelease, error) {
	cs := c.MustGet("cluster").(*cluster.ClientSet)
	cfg, err := h.actionConfigForClientSet(cs, helmutil.StorageNamespace(namespace))
	if err != nil {
		return nil, err
	}
	rel, err := helmutil.GetRelease(cfg, name)
	if err != nil {
		return nil, err
	}
	hr := helmutil.ToHelmRelease(rel, details)
	return &hr, nil
}

func (h *HelmReleaseHandler) actionConfig(c *gin.Context, namespace string) (*action.Configuration, error) {
	cs := c.MustGet("cluster").(*cluster.ClientSet)
	return h.actionConfigForClientSet(cs, helmutil.StorageNamespace(namespace))
}

func (h *HelmReleaseHandler) actionConfigForClientSet(cs *cluster.ClientSet, namespace string) (*action.Configuration, error) {
	return helmutil.NewActionConfig(cs.K8sClient.Configuration, namespace)
}

func helmReleaseID(release helmutil.HelmRelease) string {
	if release.UID != "" {
		return string(release.UID)
	}
	return release.Namespace + "/" + release.Name
}
