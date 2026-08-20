package resources

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

func writeKubernetesError(c *gin.Context, err error, prefix string) {
	statusCode := kubernetesStatusCode(err, http.StatusInternalServerError)
	message := err.Error()
	if prefix != "" {
		message = fmt.Sprintf("%s: %s", prefix, message)
	}
	c.JSON(statusCode, gin.H{"error": message})
}

func kubernetesStatusCode(err error, fallback int) int {
	var apiStatus apierrors.APIStatus
	if errors.As(err, &apiStatus) {
		code := int(apiStatus.Status().Code)
		if code >= 400 && code <= 599 {
			return code
		}
	}
	return fallback
}
