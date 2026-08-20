package helm

import (
	"net/http"
	"testing"
	"time"
)

func TestHelmCatalogCachesAreBoundedAndExpire(t *testing.T) {
	now := time.Now()
	indexCache := map[string]cachedRepositoryIndex{
		"expired": {expiresAt: now.Add(-time.Second)},
		"old":     {expiresAt: now.Add(time.Minute)},
		"new":     {expiresAt: now.Add(2 * time.Minute)},
	}
	pruneRepositoryIndexCache(indexCache, now)
	evictRepositoryIndexCache(indexCache, 2)
	if len(indexCache) != 1 || indexCache["new"].expiresAt.IsZero() {
		t.Fatalf("index cache = %#v, want only newest entry before insertion", indexCache)
	}

	contentCache := map[string]cachedChartContent{
		"expired": {expiresAt: now.Add(-time.Second)},
		"old":     {expiresAt: now.Add(time.Minute)},
		"new":     {expiresAt: now.Add(2 * time.Minute)},
	}
	pruneChartContentCache(contentCache, now)
	evictChartContentCache(contentCache, 2)
	if len(contentCache) != 1 || contentCache["new"].expiresAt.IsZero() {
		t.Fatalf("content cache = %#v, want only newest entry before insertion", contentCache)
	}
}

func TestArtifactHubCacheIsBoundedAndExpires(t *testing.T) {
	now := time.Now()
	artifactHubCacheMu.Lock()
	original := artifactHubCache
	defer func() {
		artifactHubCache = original
		artifactHubCacheMu.Unlock()
	}()
	artifactHubCache = map[string]cachedArtifactHubResponse{
		"expired": {headers: http.Header{}, expiresAt: now.Add(-time.Second)},
		"old":     {headers: http.Header{}, expiresAt: now.Add(time.Minute)},
		"new":     {headers: http.Header{}, expiresAt: now.Add(2 * time.Minute)},
	}
	pruneArtifactHubCache(now)
	evictArtifactHubCache(2)
	if len(artifactHubCache) != 1 || artifactHubCache["new"].expiresAt.IsZero() {
		t.Fatalf("artifact hub cache = %#v, want only newest entry before insertion", artifactHubCache)
	}
}
