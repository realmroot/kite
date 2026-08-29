package helmutil

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/realmroot/lightkite/pkg/model"
)

func TestLoadArchiveContextRejectsDeclaredOversize(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "52428801")
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	_, err := LoadArchiveContext(context.Background(), server.URL+"/large.tgz", nil)
	if err == nil || !strings.Contains(err.Error(), "chart archive exceeds") {
		t.Fatalf("LoadArchiveContext() error = %v, want size limit", err)
	}
}

func TestLoadArchiveContextPropagatesCancellation(t *testing.T) {
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
	}))
	t.Cleanup(server.Close)

	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, err := LoadArchiveContext(ctx, server.URL+"/cancel.tgz", nil)
		result <- err
	}()
	<-started
	cancel()
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("LoadArchiveContext() error = %v, want context.Canceled", err)
	}
}

func TestChartRedirectDoesNotForwardRepositoryCredentialsCrossOrigin(t *testing.T) {
	redirectAuthorization := make(chan string, 1)
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirectAuthorization <- r.Header.Get("Authorization")
		_, _ = w.Write([]byte("not a chart"))
	}))
	t.Cleanup(target.Close)

	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/chart.tgz", http.StatusFound)
	}))
	t.Cleanup(source.Close)
	repository := &model.HelmRepository{
		URL:      source.URL,
		Username: "repo-user",
		Password: "repo-password",
	}

	_, _ = LoadArchiveContext(context.Background(), source.URL+"/chart.tgz", repository)
	if got := <-redirectAuthorization; got != "" {
		t.Fatalf("cross-origin redirect Authorization = %q, want empty", got)
	}
}

func TestArchiveCacheIsBoundedAndExpires(t *testing.T) {
	now := time.Now()
	cache := map[string]cachedArchive{
		"expired": {expiresAt: now.Add(-time.Second)},
		"old":     {expiresAt: now.Add(time.Minute)},
		"new":     {expiresAt: now.Add(2 * time.Minute)},
	}
	archiveCacheMu.Lock()
	original := archiveCache
	defer func() {
		archiveCache = original
		archiveCacheMu.Unlock()
	}()
	archiveCache = cache
	pruneArchiveCache(now)
	evictArchiveCache(2)
	if len(archiveCache) != 1 || archiveCache["new"].expiresAt.IsZero() {
		t.Fatalf("archive cache = %#v, want only newest entry before insertion", archiveCache)
	}
}
