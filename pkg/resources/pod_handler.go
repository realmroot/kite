package resources

import (
	"fmt"
	"mime"
	"net/http"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/realmroot/lightkite/pkg/cluster"
	"github.com/realmroot/lightkite/pkg/common"
	"github.com/realmroot/lightkite/pkg/kube"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
)

type PodHandler struct {
	*GenericResourceHandler[*corev1.Pod, *corev1.PodList]
}

func NewPodHandler() *PodHandler {
	return &PodHandler{GenericResourceHandler: NewGenericResourceHandler[*corev1.Pod, *corev1.PodList](common.Pods)}
}

func (h *PodHandler) registerCustomRoutes(group *gin.RouterGroup) {
	files := group.Group("/:namespace/:name/files")
	files.GET("", h.ListFiles)
	files.GET("/preview", h.PreviewFile)
	files.GET("/download", h.DownloadFile)
	files.PUT("/upload", h.UploadFile)
}

type FileInfo struct {
	Name    string `json:"name"`
	IsDir   bool   `json:"isDir"`
	Size    string `json:"size"`
	ModTime string `json:"modTime"`
	Mode    string `json:"mode"`
	UID     string `json:"uid,omitempty"`
	GID     string `json:"gid,omitempty"`
}

func (h *PodHandler) ListFiles(c *gin.Context) {
	namespace, podName, container := c.Param("namespace"), c.Param("name"), c.Query("container")
	path := c.Query("path")
	if path == "" {
		path = "/"
	}
	cs := c.MustGet("cluster").(*cluster.ClientSet)
	stdout, stderr, err := cs.K8sClient.ExecCommandBuffered(c.Request.Context(), namespace, podName, container, []string{"ls", "-lah", "--full-time", path})
	if err != nil {
		if strings.Contains(err.Error(), "not found") || strings.Contains(stderr, "not found") {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("File browsing is not supported for %s container (missing 'ls' command)", container)})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, parseLsOutput(stdout))
}

func parseLsOutput(output string) []FileInfo {
	files := make([]FileInfo, 0)
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, "total") || strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 9 {
			continue
		}
		name := strings.Join(parts[8:], " ")
		if name == "." || name == ".." {
			continue
		}
		files = append(files, FileInfo{
			Name: name, IsDir: strings.HasPrefix(parts[0], "d"), Size: parts[4],
			ModTime: strings.Join(parts[5:7], " "), Mode: parts[0], UID: parts[2], GID: parts[3],
		})
	}
	sort.Slice(files, func(i, j int) bool {
		if files[i].IsDir != files[j].IsDir {
			return files[i].IsDir
		}
		return strings.ToLower(files[i].Name) < strings.ToLower(files[j].Name)
	})
	return files
}

func cleanFilePath(c *gin.Context) (string, bool) {
	path := c.Query("path")
	if path == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path is required"})
		return "", false
	}
	if strings.Contains(path, "->") {
		path = strings.TrimSpace(strings.SplitN(path, "->", 2)[0])
	}
	return path, true
}

func (h *PodHandler) PreviewFile(c *gin.Context) {
	path, ok := cleanFilePath(c)
	if !ok {
		return
	}
	contentType := mime.TypeByExtension(filepath.Ext(path))
	if contentType == "" {
		contentType = "text/plain; charset=utf-8"
	}
	c.Header("Content-Type", contentType)
	c.Header("Content-Disposition", fmt.Sprintf("inline; filename=\"%s\"", filepath.Base(path)))
	cs := c.MustGet("cluster").(*cluster.ClientSet)
	if err := cs.K8sClient.ExecCommand(c.Request.Context(), kube.ExecOptions{
		Namespace: c.Param("namespace"), PodName: c.Param("name"), ContainerName: c.Query("container"),
		Command: []string{"cat", path}, Stdout: c.Writer,
	}); err != nil {
		klog.Errorf("Failed to preview file: %v", err)
	}
}

func (h *PodHandler) DownloadFile(c *gin.Context) {
	path, ok := cleanFilePath(c)
	if !ok {
		return
	}
	cs := c.MustGet("cluster").(*cluster.ClientSet)
	namespace, podName, container := c.Param("namespace"), c.Param("name"), c.Query("container")
	_, _, directoryError := cs.K8sClient.ExecCommandBuffered(c.Request.Context(), namespace, podName, container, []string{"test", "-d", path})
	command := []string{"cat", path}
	if directoryError == nil {
		c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s.tar\"", filepath.Base(path)))
		c.Header("Content-Type", "application/x-tar")
		command = []string{"tar", "cf", "-", path}
	} else {
		c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filepath.Base(path)))
		c.Header("Content-Type", "application/octet-stream")
	}
	if err := cs.K8sClient.ExecCommand(c.Request.Context(), kube.ExecOptions{
		Namespace: namespace, PodName: podName, ContainerName: container, Command: command, Stdout: c.Writer,
	}); err != nil {
		klog.Errorf("Failed to download file: %v", err)
	}
}

func (h *PodHandler) UploadFile(c *gin.Context) {
	path := c.Query("path")
	if path == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path is required"})
		return
	}
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to get file from request"})
		return
	}
	defer func() {
		if err := file.Close(); err != nil {
			klog.Errorf("failed to close uploaded file: %v", err)
		}
	}()
	filename := filepath.Base(header.Filename)
	if filename == "." || filename == ".." || strings.Contains(filename, "/") || strings.Contains(filename, "\\") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid filename"})
		return
	}
	cs := c.MustGet("cluster").(*cluster.ClientSet)
	err = cs.K8sClient.ExecCommand(c.Request.Context(), kube.ExecOptions{
		Namespace: c.Param("namespace"), PodName: c.Param("name"), ContainerName: c.Query("container"),
		Command: []string{"tee", filepath.Join(path, filename)}, Stdin: file,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to upload file: %v", err)})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "file uploaded successfully"})
}
