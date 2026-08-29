package helmutil

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	semver "github.com/blang/semver/v4"
	"github.com/realmroot/lightkite/pkg/model"
	repo "helm.sh/helm/v4/pkg/repo/v1"
)

const (
	ChartSourceRepository  = "repository"
	ChartSourceArtifactHub = "artifacthub"

	artifactHubHelmPackageAPIURL = "https://artifacthub.io/api/v1/packages/helm/"
	maxArtifactHubPackageBytes   = 1 << 20
	maxRepositoryIndexBytes      = 20 << 20
)

type ChartPackage struct {
	Version    string
	URL        string
	Repository *model.HelmRepository
}

type artifactHubPackage struct {
	Version    string `json:"version"`
	ContentURL string `json:"content_url"`
}

func LatestChartPackage(ctx context.Context, source, repositoryName, chartName string) (ChartPackage, error) {
	return ResolveChartPackage(ctx, source, repositoryName, chartName, "")
}

// ResolveChartPackage resolves a chart package from a server-known catalog
// identity. Callers never need to trust a client-supplied download URL.
func ResolveChartPackage(ctx context.Context, source, repositoryName, chartName, version string) (ChartPackage, error) {
	switch source {
	case "", ChartSourceRepository:
		return repositoryChartPackage(ctx, repositoryName, chartName, version)
	case ChartSourceArtifactHub:
		return artifactHubChartPackage(ctx, repositoryName, chartName, version)
	default:
		return ChartPackage{}, fmt.Errorf("unsupported chart source")
	}
}

func repositoryChartPackage(ctx context.Context, repositoryName, chartName, version string) (ChartPackage, error) {
	var repository model.HelmRepository
	if err := model.DB.Where("name = ?", repositoryName).First(&repository).Error; err != nil {
		return ChartPackage{}, err
	}
	indexFile, err := LoadRepositoryIndexContext(ctx, repository)
	if err != nil {
		return ChartPackage{}, err
	}
	selected, err := indexFile.Get(chartName, version)
	if err != nil {
		return ChartPackage{}, fmt.Errorf("chart not found: %w", err)
	}
	if version == "" {
		versions := indexFile.Entries[chartName]
		for _, candidate := range versions {
			if CompareChartVersions(candidate.Version, selected.Version) > 0 {
				selected = candidate
			}
		}
	}
	if len(selected.URLs) == 0 {
		return ChartPackage{}, fmt.Errorf("chart package URL is missing")
	}
	return ChartPackage{
		Version:    selected.Version,
		URL:        ResolveURL(repository.URL, selected.URLs[0]),
		Repository: &repository,
	}, nil
}

func LoadRepositoryIndexContext(ctx context.Context, repository model.HelmRepository) (*repo.IndexFile, error) {
	indexURL, err := repo.ResolveReferenceURL(repository.URL, "index.yaml")
	if err != nil {
		return nil, err
	}
	data, err := downloadHTTPResource(
		ctx,
		indexURL,
		&repository,
		maxRepositoryIndexBytes,
		"application/x-yaml,text/yaml,text/plain",
		"repository index",
	)
	if err != nil {
		return nil, err
	}
	indexFile, err := os.CreateTemp("", "lightkite-helm-index-*.yaml")
	if err != nil {
		return nil, err
	}
	indexPath := indexFile.Name()
	defer func() { _ = os.Remove(indexPath) }()
	if _, err := indexFile.Write(data); err != nil {
		_ = indexFile.Close()
		return nil, err
	}
	if err := indexFile.Close(); err != nil {
		return nil, err
	}
	return repo.LoadIndexFile(indexPath)
}

func artifactHubChartPackage(ctx context.Context, repositoryName, chartName, version string) (ChartPackage, error) {
	packageURL := artifactHubHelmPackageAPIURL + url.PathEscape(repositoryName) + "/" + url.PathEscape(chartName)
	if version != "" {
		packageURL += "/" + url.PathEscape(version)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, packageURL, nil)
	if err != nil {
		return ChartPackage{}, err
	}
	req.Header.Set("User-Agent", "lightkite")

	client := http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return ChartPackage{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return ChartPackage{}, fmt.Errorf("artifact hub request failed: %s", resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxArtifactHubPackageBytes+1))
	if err != nil {
		return ChartPackage{}, err
	}
	if len(data) > maxArtifactHubPackageBytes {
		return ChartPackage{}, fmt.Errorf("artifact hub package response exceeds %d bytes", maxArtifactHubPackageBytes)
	}
	var pkg artifactHubPackage
	if err := json.Unmarshal(data, &pkg); err != nil {
		return ChartPackage{}, err
	}
	if strings.TrimSpace(pkg.ContentURL) == "" {
		return ChartPackage{}, fmt.Errorf("chart package URL is missing")
	}
	return ChartPackage{
		Version: pkg.Version,
		URL:     pkg.ContentURL,
	}, nil
}

func IsChartVersionNewer(next, current string) bool {
	return CompareChartVersions(next, current) > 0
}

func CompareChartVersions(a, b string) int {
	parsedA, errA := semver.ParseTolerant(a)
	parsedB, errB := semver.ParseTolerant(b)
	if errA == nil && errB == nil {
		return parsedA.Compare(parsedB)
	}
	if errA == nil {
		return 1
	}
	if errB == nil {
		return -1
	}
	return strings.Compare(a, b)
}
