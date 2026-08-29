package search

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/realmroot/lightkite/pkg/common"
	"github.com/realmroot/lightkite/pkg/middleware"
	"github.com/realmroot/lightkite/pkg/model"
	"github.com/realmroot/lightkite/pkg/resources"
)

func TestSearchPropagatesRequestCancellation(t *testing.T) {
	var canceled atomic.Bool
	handler := NewSearchHandler(map[string]resources.SearchFunc{
		"pods": func(c *gin.Context, _ string, _ int64) ([]common.SearchResult, error) {
			<-c.Request.Context().Done()
			canceled.Store(true)
			return nil, c.Request.Context().Err()
		},
	})
	requestContext, cancel := context.WithCancel(context.Background())
	ctx := newSearchContext(t, "cluster-a")
	ctx.Request = ctx.Request.WithContext(requestContext)
	cancel()

	if _, err := handler.Search(ctx, "target", 10); err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if !canceled.Load() {
		t.Fatal("search function did not observe request cancellation")
	}
}

func TestNormalizeSearchQuery(t *testing.T) {
	got := normalizeSearchQuery("  pod   target\t\n")
	want := "pod target"
	if got != want {
		t.Fatalf("normalizeSearchQuery() = %q, want %q", got, want)
	}
}

func TestNormalizeSearchLimit(t *testing.T) {
	tests := []struct {
		name  string
		input int
		want  int
	}{
		{name: "valid lower bound", input: 1, want: 1},
		{name: "valid upper bound", input: 100, want: 100},
		{name: "zero defaults", input: 0, want: defaultSearchLimit},
		{name: "negative defaults", input: -1, want: defaultSearchLimit},
		{name: "too large defaults", input: 101, want: defaultSearchLimit},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeSearchLimit(tt.input)
			if got != tt.want {
				t.Fatalf("normalizeSearchLimit(%d) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestSortResults(t *testing.T) {
	results := []common.SearchResult{
		{Name: "pod-1", ResourceType: "pods"},
		{Name: "target", ResourceType: "namespaces"},
		{Name: "target", ResourceType: "deployments"},
		{Name: "target-x", ResourceType: "services"},
	}

	sortResults(results, "target")

	if results[0].Name != "target" || results[0].ResourceType != "deployments" {
		t.Fatalf("first result mismatch: got %s/%s", results[0].Name, results[0].ResourceType)
	}
	if results[1].Name != "target" || results[1].ResourceType != "namespaces" {
		t.Fatalf("second result mismatch: got %s/%s", results[1].Name, results[1].ResourceType)
	}
}

func TestGlobalSearchNegativeLimitDoesNotPanic(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/search?q=po&limit=-1", nil)
	ctx.Set("user", model.User{Username: "test-user"})

	handler := NewSearchHandler(map[string]resources.SearchFunc{})

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("GlobalSearch panicked with negative limit: %v", r)
		}
	}()

	handler.GlobalSearch(ctx)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status code: got %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestGlobalSearchUsesCurrentClusterAndLimit(t *testing.T) {
	searchFuncs := map[string]resources.SearchFunc{
		"pods": func(c *gin.Context, _ string, _ int64) ([]common.SearchResult, error) {
			clusterName := c.GetString(middleware.ClusterNameKey)
			switch clusterName {
			case "cluster-a":
				return []common.SearchResult{
					{Name: "target-a-1", ResourceType: "pods"},
					{Name: "target-a-2", ResourceType: "pods"},
					{Name: "target-a-3", ResourceType: "pods"},
				}, nil
			case "cluster-b":
				return []common.SearchResult{
					{Name: "target-b-1", ResourceType: "pods"},
					{Name: "target-b-2", ResourceType: "pods"},
				}, nil
			default:
				return nil, fmt.Errorf("unexpected cluster: %s", clusterName)
			}
		},
	}

	handler := NewSearchHandler(searchFuncs)

	ctx := newSearchContext(t, "cluster-a")
	if _, err := handler.Search(ctx, "po target", 1); err != nil {
		t.Fatalf("Search returned error: %v", err)
	}

	resp := performGlobalSearch(t, handler, "cluster-a", "/search?q=po+target&limit=3")
	if resp.Total != 3 {
		t.Fatalf("cluster/limit search returned %d results, want 3", resp.Total)
	}

	resp = performGlobalSearch(t, handler, "cluster-b", "/search?q=po+target&limit=3")
	if resp.Total != 2 {
		t.Fatalf("cluster-specific search returned %d results, want 2", resp.Total)
	}
	if len(resp.Results) == 0 || resp.Results[0].Name != "target-b-1" {
		t.Fatalf("unexpected cluster-b results: %#v", resp.Results)
	}
}

func newSearchContext(t *testing.T, clusterName string) *gin.Context {
	t.Helper()

	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/search", nil)
	if clusterName != "" {
		ctx.Set(middleware.ClusterNameKey, clusterName)
	}
	ctx.Set("user", model.User{Username: "test-user"})
	return ctx
}

func performGlobalSearch(t *testing.T, handler *SearchHandler, clusterName, target string) SearchResponse {
	return performGlobalSearchForUser(t, handler, clusterName, target, model.User{Username: "test-user"})
}

func performGlobalSearchForUser(t *testing.T, handler *SearchHandler, clusterName, target string, user model.User) SearchResponse {
	t.Helper()

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodGet, target, nil)
	if clusterName != "" {
		ctx.Set(middleware.ClusterNameKey, clusterName)
	}
	ctx.Set("user", user)

	handler.GlobalSearch(ctx)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status code: got %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var resp SearchResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	return resp
}

func TestGlobalSearchRechecksKubernetesForEveryRequest(t *testing.T) {
	var calls atomic.Int32
	handler := NewSearchHandler(map[string]resources.SearchFunc{
		string(common.Nodes): func(_ *gin.Context, _ string, _ int64) ([]common.SearchResult, error) {
			calls.Add(1)
			return []common.SearchResult{{Name: "worker-node", ResourceType: string(common.Nodes)}}, nil
		},
	})
	allowedUser := model.User{Issuer: "https://issuer.example", Sub: "alice-subject", Username: "alice"}
	response := performGlobalSearchForUser(t, handler, "cluster-a", "/search?q=worker&limit=50", allowedUser)
	if response.Total != 1 {
		t.Fatalf("initial search returned %#v, want one node", response.Results)
	}

	revokedUser := model.User{Issuer: "https://issuer.example", Sub: "alice-subject", Username: "alice"}
	response = performGlobalSearchForUser(t, handler, "cluster-a", "/search?q=worker&limit=50", revokedUser)
	if response.Total != 1 {
		t.Fatalf("second authorized search returned %#v", response.Results)
	}
	if calls.Load() != 2 {
		t.Fatalf("search function called %d times, want Kubernetes recheck", calls.Load())
	}
}

func TestGlobalSearchUsesCurrentAuthenticatedUser(t *testing.T) {
	var calls atomic.Int32
	handler := NewSearchHandler(map[string]resources.SearchFunc{
		string(common.Nodes): func(c *gin.Context, _ string, _ int64) ([]common.SearchResult, error) {
			calls.Add(1)
			user := c.MustGet("user").(model.User)
			return []common.SearchResult{{Name: user.Sub, ResourceType: string(common.Nodes)}}, nil
		},
	})
	first := model.User{Issuer: "https://issuer.example", Sub: "subject-one", Username: "shared@example.com"}
	second := model.User{Issuer: "https://issuer.example", Sub: "subject-two", Username: "shared@example.com"}

	response := performGlobalSearchForUser(t, handler, "cluster-a", "/search?q=subject&limit=50", first)
	if response.Results[0].Name != first.Sub {
		t.Fatalf("first identity results = %#v", response.Results)
	}
	response = performGlobalSearchForUser(t, handler, "cluster-a", "/search?q=subject&limit=50", second)
	if response.Results[0].Name != second.Sub {
		t.Fatalf("second identity received another subject's results: %#v", response.Results)
	}
	if calls.Load() != 2 {
		t.Fatalf("search calls = %d, want separate query per OIDC subject", calls.Load())
	}
}

func TestGlobalSearchTrustsKubernetesAuthorizedSearchResults(t *testing.T) {
	handler := NewSearchHandler(map[string]resources.SearchFunc{
		string(common.Nodes): func(_ *gin.Context, _ string, _ int64) ([]common.SearchResult, error) {
			return []common.SearchResult{{Name: "worker-node", ResourceType: string(common.Nodes)}}, nil
		},
	})
	user := model.User{Username: "alice"}
	response := performGlobalSearchForUser(t, handler, "cluster-a", "/search?q=worker&limit=50", user)
	if response.Total != 1 {
		t.Fatalf("search result was filtered by application roles: %#v", response.Results)
	}
}

// TestSearchParallelExecution verifies that multiple resource searches run concurrently.
func TestSearchParallelExecution(t *testing.T) {
	// Track concurrent execution: each func sleeps and records max concurrency.
	var running atomic.Int32
	var maxConcurrent atomic.Int32

	slowSearch := func(results []common.SearchResult) resources.SearchFunc {
		return func(_ *gin.Context, _ string, _ int64) ([]common.SearchResult, error) {
			cur := running.Add(1)
			// Update max concurrency seen
			for {
				old := maxConcurrent.Load()
				if cur <= old || maxConcurrent.CompareAndSwap(old, cur) {
					break
				}
			}
			time.Sleep(50 * time.Millisecond)
			running.Add(-1)
			return results, nil
		}
	}

	searchFuncs := map[string]resources.SearchFunc{
		"pods":        slowSearch([]common.SearchResult{{Name: "nginx", ResourceType: "pods"}}),
		"services":    slowSearch([]common.SearchResult{{Name: "nginx-svc", ResourceType: "services"}}),
		"deployments": slowSearch([]common.SearchResult{{Name: "nginx-deploy", ResourceType: "deployments"}}),
	}

	handler := NewSearchHandler(searchFuncs)
	ctx := newSearchContext(t, "test-cluster")

	start := time.Now()
	results, err := handler.Search(ctx, "nginx", 50)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}

	// With 3 funcs sleeping 50ms each, sequential would take >= 150ms.
	// Parallel should complete in ~50-80ms. Allow generous margin.
	if elapsed >= 140*time.Millisecond {
		t.Errorf("Search took %v, expected < 140ms for parallel execution", elapsed)
	}

	if maxConcurrent.Load() < 2 {
		t.Errorf("maxConcurrent = %d, want >= 2 (proves parallelism)", maxConcurrent.Load())
	}

	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
}

// TestSearchPartialFailure ensures that one failing resource type doesn't break others.
func TestSearchPartialFailure(t *testing.T) {
	var callCount atomic.Int32
	searchFuncs := map[string]resources.SearchFunc{
		"pods": func(_ *gin.Context, _ string, _ int64) ([]common.SearchResult, error) {
			callCount.Add(1)
			return []common.SearchResult{{Name: "ok-pod", ResourceType: "pods"}}, nil
		},
		"services": func(_ *gin.Context, _ string, _ int64) ([]common.SearchResult, error) {
			callCount.Add(1)
			return nil, fmt.Errorf("simulated API server error")
		},
		"deployments": func(_ *gin.Context, _ string, _ int64) ([]common.SearchResult, error) {
			callCount.Add(1)
			return []common.SearchResult{{Name: "ok-deploy", ResourceType: "deployments"}}, nil
		},
	}

	handler := NewSearchHandler(searchFuncs)
	ctx := newSearchContext(t, "test-cluster")

	results, err := handler.Search(ctx, "ok", 50)
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}

	// Should have results from pods + deployments (services failed gracefully)
	if len(results) != 2 {
		t.Fatalf("expected 2 results (failed resource skipped), got %d: %+v", len(results), results)
	}

	callsBefore := callCount.Load()

	// Every call rechecks the API server, including after a partial failure.
	ctx2 := newSearchContext(t, "test-cluster")
	results2, err := handler.Search(ctx2, "ok", 50)
	if err != nil {
		t.Fatalf("second Search returned error: %v", err)
	}
	if len(results2) != 2 {
		t.Fatalf("expected 2 results on retry, got %d", len(results2))
	}

	callsAfter := callCount.Load()
	if callsAfter == callsBefore {
		t.Fatal("second call did not recheck Kubernetes after a partial failure")
	}
}

// TestSearchPanicReturnsPartialResults verifies that one buggy resource search
// does not suppress the authorized results returned by other resource types.
func TestSearchPanicReturnsPartialResults(t *testing.T) {
	var callCount atomic.Int32

	searchFuncs := map[string]resources.SearchFunc{
		"pods": func(_ *gin.Context, _ string, _ int64) ([]common.SearchResult, error) {
			callCount.Add(1)
			return []common.SearchResult{{Name: "ok-pod", ResourceType: "pods"}}, nil
		},
		"services": func(_ *gin.Context, _ string, _ int64) ([]common.SearchResult, error) {
			callCount.Add(1)
			panic("simulated nil-pointer in service search")
		},
	}

	handler := NewSearchHandler(searchFuncs)

	// First call: one function panics and partial results are returned.
	ctx1 := newSearchContext(t, "test-cluster")
	results, err := handler.Search(ctx1, "ok", 50)
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if len(results) != 1 || results[0].Name != "ok-pod" {
		t.Fatalf("expected partial result [ok-pod], got %+v", results)
	}

	callsBefore := callCount.Load()

	// Both search functions must be invoked again on the next request.
	ctx2 := newSearchContext(t, "test-cluster")
	results2, err := handler.Search(ctx2, "ok", 50)
	if err != nil {
		t.Fatalf("second Search returned error: %v", err)
	}
	if len(results2) != 1 {
		t.Fatalf("expected 1 result on retry, got %d", len(results2))
	}

	callsAfter := callCount.Load()
	if callsAfter == callsBefore {
		t.Fatal("second call did not recheck Kubernetes after a panic")
	}
}

// TestSearchEmptyResourceFuncs verifies Search handles zero searchable types gracefully.
func TestSearchEmptyResourceFuncs(t *testing.T) {
	handler := NewSearchHandler(map[string]resources.SearchFunc{})
	ctx := newSearchContext(t, "test-cluster")

	results, err := handler.Search(ctx, "anything", 50)
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected 0 results with no search funcs, got %d", len(results))
	}
}
