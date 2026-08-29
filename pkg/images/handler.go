package images

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/distribution/reference"
	"github.com/gin-gonic/gin"
)

const (
	registryRequestTimeout  = 10 * time.Second
	maxRegistryResponseSize = 2 << 20
)

var defaultRegistryHosts = map[string]struct{}{
	"docker.io":       {},
	"hub.docker.com":  {},
	"ghcr.io":         {},
	"quay.io":         {},
	"registry.k8s.io": {},
	"gcr.io":          {},
}

type ImageTagInfo struct {
	Name      string     `json:"name"`
	Timestamp *time.Time `json:"timestamp,omitempty"`
}

type registry interface {
	GetTags(ctx context.Context) ([]ImageTagInfo, error)
}

type dockerRegistry struct {
	repo string
}

func (d dockerRegistry) GetTags(ctx context.Context) ([]ImageTagInfo, error) {
	url := fmt.Sprintf("https://hub.docker.com/v2/repositories/%s/tags?page_size=10&ordering=last_updated", d.repo)
	resp, err := getRegistryResponse(ctx, url)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("docker hub tag request failed: %s", resp.Status)
	}
	var data struct {
		Results []struct {
			Name        string    `json:"name"`
			LastUpdated time.Time `json:"last_updated"`
		}
	}
	if err := decodeRegistryJSON(resp, &data); err != nil {
		return nil, err
	}
	tags := make([]ImageTagInfo, 0, len(data.Results))
	for _, t := range data.Results {
		tags = append(tags, ImageTagInfo{Name: t.Name, Timestamp: &t.LastUpdated})
	}
	return tags, nil
}

type containerRegistryV2 struct {
	baseURL string
	repo    string
}

func (d containerRegistryV2) GetTags(ctx context.Context) ([]ImageTagInfo, error) {
	url := fmt.Sprintf("https://%s/v2/%s/tags/list", d.baseURL, d.repo)
	resp, err := getRegistryResponse(ctx, url)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("registry tag request failed: %s", resp.Status)
	}
	var data struct {
		Tags []string `json:"tags"`
	}
	if err := decodeRegistryJSON(resp, &data); err != nil {
		return nil, err
	}
	tags := make([]ImageTagInfo, 0, len(data.Tags))
	for _, t := range data.Tags {
		if strings.HasPrefix(t, "sha256") {
			// Skip digest tags
			continue
		}
		tags = append(tags, ImageTagInfo{Name: t})
	}
	return tags, nil
}

func getRegistry(image string) (registry, error) {
	named, err := reference.ParseNormalizedNamed(strings.TrimSpace(image))
	if err != nil {
		return nil, fmt.Errorf("invalid image reference: %w", err)
	}
	r, repo := reference.Domain(named), reference.Path(named)
	if !registryHostAllowed(r) {
		return nil, fmt.Errorf("registry host %q is not enabled by the operator", r)
	}
	if r == "docker.io" {
		return dockerRegistry{repo}, nil
	}
	return containerRegistryV2{baseURL: r, repo: repo}, nil
}

func registryHostAllowed(host string) bool {
	host = strings.ToLower(host)
	if _, allowed := defaultRegistryHosts[host]; allowed {
		return true
	}
	for _, configured := range strings.Split(os.Getenv("KITE_IMAGE_REGISTRY_HOSTS"), ",") {
		if strings.EqualFold(strings.TrimSpace(configured), host) {
			return true
		}
	}
	return false
}

func getRegistryResponse(ctx context.Context, targetURL string) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "lightkite")
	client := &http.Client{
		Timeout: registryRequestTimeout,
		CheckRedirect: func(next *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("registry request stopped after 10 redirects")
			}
			if !registryHostAllowed(next.URL.Host) {
				return fmt.Errorf("registry redirect host %q is not enabled by the operator", next.URL.Host)
			}
			return nil
		},
	}
	return client.Do(request)
}

func decodeRegistryJSON(response *http.Response, target any) error {
	if response.ContentLength > maxRegistryResponseSize {
		return fmt.Errorf("registry response exceeds %d bytes", maxRegistryResponseSize)
	}
	limited := io.LimitReader(response.Body, maxRegistryResponseSize+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return err
	}
	if len(data) > maxRegistryResponseSize {
		return fmt.Errorf("registry response exceeds %d bytes", maxRegistryResponseSize)
	}
	if err := json.Unmarshal(data, target); err != nil {
		return fmt.Errorf("decode registry response: %w", err)
	}
	return nil
}

func GetImageTags(c *gin.Context) {
	image := c.Query("image")
	if image == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "image param required"})
		return
	}
	reg, err := getRegistry(image)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	tags, err := reg.GetTags(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, tags)
}
