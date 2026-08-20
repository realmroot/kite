package resources

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"helm.sh/helm/v4/pkg/storage/driver"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestWriteKubernetesErrorPreservesAPIStatus(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{"forbidden", apierrors.NewForbidden(schema.GroupResource{Resource: "pods"}, "demo", errors.New("denied")), http.StatusForbidden},
		{"not found", apierrors.NewNotFound(schema.GroupResource{Resource: "pods"}, "demo"), http.StatusNotFound},
		{"internal", errors.New("transport failed"), http.StatusInternalServerError},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(recorder)
			writeKubernetesError(context, test.err, "request failed")
			if recorder.Code != test.want {
				t.Fatalf("status = %d, want %d: %s", recorder.Code, test.want, recorder.Body.String())
			}
		})
	}
}

func TestHelmErrorStatusPreservesKubernetesAndReleaseSemantics(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{"forbidden", apierrors.NewForbidden(schema.GroupResource{Resource: "secrets"}, "release", errors.New("denied")), http.StatusForbidden},
		{"release not found", driver.ErrReleaseNotFound, http.StatusNotFound},
		{"internal", errors.New("helm failed"), http.StatusInternalServerError},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := helmErrorStatus(test.err); got != test.want {
				t.Fatalf("helmErrorStatus() = %d, want %d", got, test.want)
			}
		})
	}
}
