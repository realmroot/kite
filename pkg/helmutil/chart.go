package helmutil

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/zxh326/kite/pkg/model"
	chart "helm.sh/helm/v4/pkg/chart/v2"
	"helm.sh/helm/v4/pkg/chart/v2/loader"
	"helm.sh/helm/v4/pkg/getter"
	"helm.sh/helm/v4/pkg/registry"
	repo "helm.sh/helm/v4/pkg/repo/v1"
)

const (
	archiveCacheTTL        = 10 * time.Minute
	archiveCacheMaxEntries = 256
	maxChartArchiveBytes   = 50 << 20
	chartDownloadTimeout   = 30 * time.Second
)

var (
	archiveCacheMu sync.Mutex
	archiveCache   = map[string]cachedArchive{}
)

type cachedArchive struct {
	data      []byte
	expiresAt time.Time
}

func LoadRepositoryArchive(repository model.HelmRepository, entry *repo.ChartVersion) (*chart.Chart, error) {
	return LoadRepositoryArchiveContext(context.Background(), repository, entry)
}

func LoadRepositoryArchiveContext(ctx context.Context, repository model.HelmRepository, entry *repo.ChartVersion) (*chart.Chart, error) {
	if len(entry.URLs) == 0 {
		return nil, nil
	}
	chartURL, err := repo.ResolveReferenceURL(repository.URL, entry.URLs[0])
	if err != nil {
		return nil, err
	}
	return LoadArchiveContext(ctx, chartURL, &repository)
}

func LoadArchive(chartURL string, repository *model.HelmRepository) (*chart.Chart, error) {
	return LoadArchiveContext(context.Background(), chartURL, repository)
}

func LoadArchiveContext(ctx context.Context, chartURL string, repository *model.HelmRepository) (*chart.Chart, error) {
	chartURL = strings.TrimSpace(chartURL)
	parsedURL, err := url.Parse(chartURL)
	if err != nil || parsedURL.Scheme == "" {
		return nil, fmt.Errorf("chartUrl must be an absolute URL")
	}
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" && parsedURL.Scheme != "oci" {
		return nil, fmt.Errorf("unsupported chartUrl scheme")
	}

	cacheKey := archiveCacheKey(chartURL)
	now := time.Now()
	archiveCacheMu.Lock()
	cached, ok := archiveCache[cacheKey]
	if ok && now.Before(cached.expiresAt) {
		data := append([]byte(nil), cached.data...)
		archiveCacheMu.Unlock()
		return loader.LoadArchive(bytes.NewReader(data))
	}
	archiveCacheMu.Unlock()

	useRepositoryCredentials := repository != nil && repository.Username != "" && sameURLOrigin(repository.URL, chartURL)
	var archiveData []byte
	if parsedURL.Scheme == "oci" {
		client, err := getter.Getters().ByScheme(parsedURL.Scheme)
		if err != nil {
			return nil, err
		}
		options := []getter.Option{
			getter.WithAcceptHeader("application/gzip,application/octet-stream"),
			getter.WithTimeout(chartDownloadTimeout),
		}
		registryOptions := []registry.ClientOption{}
		if useRepositoryCredentials {
			registryOptions = append(registryOptions, registry.ClientOptBasicAuth(repository.Username, string(repository.Password)))
		}
		registryClient, err := registry.NewClient(registryOptions...)
		if err != nil {
			return nil, err
		}
		if !strings.Contains(path.Base(parsedURL.Path), ":") && !strings.Contains(parsedURL.Path, "@") {
			tags, err := registryClient.Tags(strings.TrimPrefix(chartURL, "oci://"))
			if err != nil {
				return nil, err
			}
			tag, err := registry.GetTagMatchingVersionOrConstraint(tags, "")
			if err != nil {
				return nil, err
			}
			chartURL = chartURL + ":" + tag
		}
		options = append(options, getter.WithRegistryClient(registryClient))
		baseURL := chartURL
		if repository != nil {
			baseURL = repository.URL
		}
		options = append(options, getter.WithURL(baseURL))
		data, err := client.Get(chartURL, options...)
		if err != nil {
			return nil, err
		}
		archiveData = data.Bytes()
		if len(archiveData) > maxChartArchiveBytes {
			return nil, fmt.Errorf("chart archive exceeds %d bytes", maxChartArchiveBytes)
		}
	} else {
		archiveData, err = downloadHTTPResource(
			ctx,
			chartURL,
			repository,
			maxChartArchiveBytes,
			"application/gzip,application/octet-stream",
			"chart archive",
		)
		if err != nil {
			return nil, err
		}
	}
	loadedChart, err := loader.LoadArchive(bytes.NewReader(archiveData))
	if err != nil {
		return nil, err
	}

	archiveCacheMu.Lock()
	pruneArchiveCache(time.Now())
	evictArchiveCache(archiveCacheMaxEntries)
	archiveCache[cacheKey] = cachedArchive{
		data:      append([]byte(nil), archiveData...),
		expiresAt: time.Now().Add(archiveCacheTTL),
	}
	archiveCacheMu.Unlock()

	return loadedChart, nil
}

func downloadHTTPResource(
	ctx context.Context,
	targetURL string,
	repository *model.HelmRepository,
	maxBytes int64,
	accept string,
	description string,
) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", accept)
	request.Header.Set("User-Agent", "kite")
	if repository != nil && repository.Username != "" && sameURLOrigin(repository.URL, targetURL) {
		request.SetBasicAuth(repository.Username, string(repository.Password))
	}
	origin := request.URL
	httpClient := &http.Client{
		Timeout: chartDownloadTimeout,
		CheckRedirect: func(next *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return errors.New("stopped after 10 redirects")
			}
			if !sameParsedURLOrigin(origin, next.URL) {
				next.Header.Del("Authorization")
			}
			return nil
		},
	}
	response, err := httpClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s download failed: %s", description, response.Status)
	}
	if response.ContentLength > maxBytes {
		return nil, fmt.Errorf("%s exceeds %d bytes", description, maxBytes)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("%s exceeds %d bytes", description, maxBytes)
	}
	return data, nil
}

func pruneArchiveCache(now time.Time) {
	for key, entry := range archiveCache {
		if !now.Before(entry.expiresAt) {
			delete(archiveCache, key)
		}
	}
}

func evictArchiveCache(limit int) {
	for len(archiveCache) >= limit {
		var oldestKey string
		var oldestExpiry time.Time
		for key, entry := range archiveCache {
			if oldestKey == "" || entry.expiresAt.Before(oldestExpiry) {
				oldestKey, oldestExpiry = key, entry.expiresAt
			}
		}
		delete(archiveCache, oldestKey)
	}
}

func ResolveURL(baseURL, refURL string) string {
	if refURL == "" {
		return ""
	}
	resolved, err := repo.ResolveReferenceURL(baseURL, refURL)
	if err != nil {
		return refURL
	}
	return resolved
}

func sameURLOrigin(baseURL, targetURL string) bool {
	base, err := url.Parse(baseURL)
	if err != nil {
		return false
	}
	target, err := url.Parse(targetURL)
	if err != nil {
		return false
	}
	return sameParsedURLOrigin(base, target)
}

func sameParsedURLOrigin(base, target *url.URL) bool {
	return strings.EqualFold(base.Scheme, target.Scheme) && strings.EqualFold(base.Host, target.Host)
}

func archiveCacheKey(chartURL string) string {
	return chartURL
}

func ClearRepositoryArchiveCache(repository model.HelmRepository) {
	cacheKey := repository.URL
	cacheKeyPrefix := strings.TrimRight(cacheKey, "/") + "/"

	archiveCacheMu.Lock()
	for key := range archiveCache {
		if key == cacheKey || strings.HasPrefix(key, cacheKeyPrefix) {
			delete(archiveCache, key)
		}
	}
	archiveCacheMu.Unlock()
}
