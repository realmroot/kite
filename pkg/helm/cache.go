package helm

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/zxh326/kite/pkg/helmutil"
	"github.com/zxh326/kite/pkg/model"
	repo "helm.sh/helm/v4/pkg/repo/v1"
)

const (
	helmRepositoryIndexCacheTTL = 5 * time.Minute
	helmChartContentCacheTTL    = 10 * time.Minute
	artifactHubCacheTTL         = 5 * time.Minute
	helmRepositoryIndexCacheMax = 128
	helmChartContentCacheMax    = 512
	artifactHubCacheMax         = 512
)

type cachedRepositoryIndex struct {
	indexFile *repo.IndexFile
	expiresAt time.Time
}

type cachedChartContent struct {
	content   helmChartContent
	expiresAt time.Time
}

type cachedArtifactHubResponse struct {
	data      []byte
	headers   http.Header
	expiresAt time.Time
}

var (
	artifactHubCacheMu sync.Mutex
	artifactHubCache   = map[string]cachedArtifactHubResponse{}
)

func (h *HelmChartHandler) loadRepositoryIndex(ctx context.Context, repository model.HelmRepository) (*repo.IndexFile, error) {
	cacheKey := repositoryIndexCacheKey(repository)
	now := time.Now()

	h.indexCacheMu.Lock()
	cached, ok := h.indexCache[cacheKey]
	if ok && now.Before(cached.expiresAt) {
		h.indexCacheMu.Unlock()
		return cached.indexFile, nil
	}
	h.indexCacheMu.Unlock()

	indexFile, err := helmutil.LoadRepositoryIndexContext(ctx, repository)
	if err != nil {
		return nil, err
	}

	h.indexCacheMu.Lock()
	pruneRepositoryIndexCache(h.indexCache, now)
	evictRepositoryIndexCache(h.indexCache, helmRepositoryIndexCacheMax)
	h.indexCache[cacheKey] = cachedRepositoryIndex{
		indexFile: indexFile,
		expiresAt: now.Add(helmRepositoryIndexCacheTTL),
	}
	h.indexCacheMu.Unlock()

	return indexFile, nil
}

func (h *HelmChartHandler) loadChartContent(ctx context.Context, repository model.HelmRepository, entry *repo.ChartVersion) (helmChartContent, error) {
	if len(entry.URLs) == 0 {
		return helmChartContent{}, nil
	}
	cacheKey := chartContentCacheKey(repository, entry)
	now := time.Now()

	h.contentCacheMu.Lock()
	cached, ok := h.contentCache[cacheKey]
	if ok && now.Before(cached.expiresAt) {
		h.contentCacheMu.Unlock()
		return cached.content, nil
	}
	h.contentCacheMu.Unlock()

	loadedChart, err := helmutil.LoadRepositoryArchiveContext(ctx, repository, entry)
	if err != nil {
		return helmChartContent{}, err
	}
	values, err := chartValues(loadedChart)
	if err != nil {
		return helmChartContent{}, err
	}
	content := helmChartContent{
		Readme:    findReadme(loadedChart.Files),
		Values:    values,
		Templates: chartTemplates(loadedChart.Templates),
	}

	h.contentCacheMu.Lock()
	pruneChartContentCache(h.contentCache, now)
	evictChartContentCache(h.contentCache, helmChartContentCacheMax)
	h.contentCache[cacheKey] = cachedChartContent{
		content:   content,
		expiresAt: now.Add(helmChartContentCacheTTL),
	}
	h.contentCacheMu.Unlock()

	return content, nil
}

func pruneRepositoryIndexCache(cache map[string]cachedRepositoryIndex, now time.Time) {
	for key, entry := range cache {
		if !now.Before(entry.expiresAt) {
			delete(cache, key)
		}
	}
}

func evictRepositoryIndexCache(cache map[string]cachedRepositoryIndex, limit int) {
	for len(cache) >= limit {
		var oldestKey string
		var oldestExpiry time.Time
		for key, entry := range cache {
			if oldestKey == "" || entry.expiresAt.Before(oldestExpiry) {
				oldestKey, oldestExpiry = key, entry.expiresAt
			}
		}
		delete(cache, oldestKey)
	}
}

func pruneChartContentCache(cache map[string]cachedChartContent, now time.Time) {
	for key, entry := range cache {
		if !now.Before(entry.expiresAt) {
			delete(cache, key)
		}
	}
}

func evictChartContentCache(cache map[string]cachedChartContent, limit int) {
	for len(cache) >= limit {
		var oldestKey string
		var oldestExpiry time.Time
		for key, entry := range cache {
			if oldestKey == "" || entry.expiresAt.Before(oldestExpiry) {
				oldestKey, oldestExpiry = key, entry.expiresAt
			}
		}
		delete(cache, oldestKey)
	}
}

func repositoryIndexCacheKey(repository model.HelmRepository) string {
	return repository.URL
}

func chartContentCacheKey(repository model.HelmRepository, entry *repo.ChartVersion) string {
	return helmutil.ResolveURL(repository.URL, entry.URLs[0])
}

func (h *HelmChartHandler) clearRepositoryCache(repository model.HelmRepository) {
	cacheKey := repositoryIndexCacheKey(repository)
	helmutil.ClearRepositoryArchiveCache(repository)

	h.indexCacheMu.Lock()
	delete(h.indexCache, cacheKey)
	h.indexCacheMu.Unlock()

	h.contentCacheMu.Lock()
	cacheKeyPrefix := strings.TrimRight(cacheKey, "/") + "/"
	for key := range h.contentCache {
		if key == cacheKey || strings.HasPrefix(key, cacheKeyPrefix) {
			delete(h.contentCache, key)
		}
	}
	h.contentCacheMu.Unlock()
}
