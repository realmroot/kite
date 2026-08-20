package search

import (
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/zxh326/kite/pkg/common"
	"github.com/zxh326/kite/pkg/resources"
	"github.com/zxh326/kite/pkg/utils"
	"golang.org/x/sync/errgroup"
	"k8s.io/klog/v2"
)

type SearchHandler struct {
	searchFuncs map[string]resources.SearchFunc
}
type SearchResponse struct {
	Results []common.SearchResult `json:"results"`
	Total   int                   `json:"total"`
}

const (
	defaultSearchLimit = 50
	maxSearchLimit     = 100
)

var searchResourceOrder = map[string]int{
	string(common.Deployments):          1,
	string(common.DaemonSets):           2,
	string(common.StatefulSets):         3,
	string(common.Pods):                 4,
	string(common.ConfigMaps):           5,
	string(common.Services):             6,
	string(common.Secrets):              7,
	string(common.Ingresses):            8,
	string(common.Namespaces):           9,
	string(common.PodDisruptionBudgets): 10,
}

func NewSearchHandler(searchFuncs map[string]resources.SearchFunc) *SearchHandler {
	return &SearchHandler{
		searchFuncs: searchFuncs,
	}
}

func (h *SearchHandler) Search(c *gin.Context, query string, limit int) ([]common.SearchResult, error) {
	start := time.Now()
	query = normalizeSearchQuery(query)
	limit = normalizeSearchLimit(limit)

	// Determine which resource types to search
	guessSearchResources, q := utils.GuessSearchResources(query)

	// Collect the search functions to execute
	type searchEntry struct {
		name string
		fn   resources.SearchFunc
	}
	var entries []searchEntry
	for name, searchFunc := range h.searchFuncs {
		if guessSearchResources == "all" || name == guessSearchResources {
			entries = append(entries, searchEntry{name: name, fn: searchFunc})
		}
	}

	// Execute searches in parallel using errgroup
	resultSlices := make([][]common.SearchResult, len(entries))
	g, searchRequestContext := errgroup.WithContext(c.Request.Context())

	for i, entry := range entries {
		searchContext := c.Copy()
		searchContext.Request = searchContext.Request.Clone(searchRequestContext)
		g.Go(func() (err error) {
			defer func() {
				if r := recover(); r != nil {
					klog.Errorf("search: resource %q panicked: %v", entry.name, r)
				}
			}()
			resourceStart := time.Now()
			results, searchErr := entry.fn(searchContext, q, int64(limit))
			elapsed := time.Since(resourceStart)
			if searchErr != nil {
				if searchRequestContext.Err() == nil {
					klog.Errorf("search: resource %q failed after %s: %v", entry.name, elapsed, searchErr)
				}
				return nil
			}
			klog.V(4).Infof("search: resource=%s query=%q results=%d elapsed=%s", entry.name, q, len(results), elapsed)
			resultSlices[i] = results
			return nil
		})
	}

	_ = g.Wait() // all goroutines return nil, error is always nil

	// Merge results from all resource types
	var allResults []common.SearchResult
	for _, slice := range resultSlices {
		allResults = append(allResults, slice...)
	}
	queryLower := strings.ToLower(q)
	sortResults(allResults, queryLower)

	// Limit total results
	if len(allResults) > limit {
		allResults = allResults[:limit]
	}

	klog.V(4).Infof("search: query=%q resources=%d results=%d elapsed=%s", query, len(entries), len(allResults), time.Since(start))
	return allResults, nil
}

// GlobalSearch handles global search across multiple resource types
func (h *SearchHandler) GlobalSearch(c *gin.Context) {
	query := normalizeSearchQuery(c.Query("q"))
	if query == "" {
		c.JSON(http.StatusOK, SearchResponse{})
		return
	}

	// Parse limit parameter
	limitStr := c.DefaultQuery("limit", strconv.Itoa(defaultSearchLimit))
	limit, err := strconv.Atoi(limitStr)
	if err != nil {
		limit = defaultSearchLimit
	}
	limit = normalizeSearchLimit(limit)

	allResults, err := h.Search(c, query, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to perform search"})
		return
	}

	response := SearchResponse{
		Results: allResults,
		Total:   len(allResults),
	}

	c.JSON(http.StatusOK, response)
}

func getResourceOrder(resourceType string) int {
	if order, exists := searchResourceOrder[resourceType]; exists {
		return order
	}
	return len(searchResourceOrder) // Default to the end if not found
}

// sortResults sorts the search results with exact matches first, then by resource type
func sortResults(results []common.SearchResult, query string) {
	var exactMatches, partialMatches []common.SearchResult

	for _, result := range results {
		if strings.ToLower(result.Name) == query {
			exactMatches = append(exactMatches, result)
		} else {
			partialMatches = append(partialMatches, result)
		}
	}

	// sort by resources
	sortByResources := func(a, b common.SearchResult) bool {
		return getResourceOrder(a.ResourceType) < getResourceOrder(b.ResourceType)
	}

	sort.SliceStable(exactMatches, func(i, j int) bool {
		return sortByResources(exactMatches[i], exactMatches[j])
	})
	sort.SliceStable(partialMatches, func(i, j int) bool {
		return sortByResources(partialMatches[i], partialMatches[j])
	})

	// Combine results
	copy(results, append(exactMatches, partialMatches...))
}

func normalizeSearchLimit(limit int) int {
	if limit < 1 || limit > maxSearchLimit {
		return defaultSearchLimit
	}
	return limit
}

func normalizeSearchQuery(query string) string {
	return strings.Join(strings.Fields(query), " ")
}
