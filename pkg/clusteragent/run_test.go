package clusteragent

import (
	"context"
	"strings"
	"testing"
)

func TestRunRequiresCredentialFreeHTTPSAPIServerMetadata(t *testing.T) {
	baseArgs := []string{
		"--server=https://kite.example.test",
		"--token=enrollment-token",
		"--public-key=registration-key",
	}
	tests := []struct {
		name      string
		args      []string
		wantError string
	}{
		{
			name:      "API server is required",
			args:      baseArgs,
			wantError: "--api-server is required",
		},
		{
			name:      "HTTP API server is rejected",
			args:      append(append([]string(nil), baseArgs...), "--api-server=http://kubernetes.default.svc"),
			wantError: "--api-server must be a valid HTTPS URL",
		},
		{
			name:      "API server URL credentials are rejected",
			args:      append(append([]string(nil), baseArgs...), "--api-server=https://admin:secret@kubernetes.default.svc"),
			wantError: "--api-server must be a valid HTTPS URL",
		},
		{
			name:      "API server URL query is rejected",
			args:      append(append([]string(nil), baseArgs...), "--api-server=https://kubernetes.default.svc?token=secret"),
			wantError: "--api-server must be a valid HTTPS URL",
		},
		{
			name:      "API server URL fragment is rejected",
			args:      append(append([]string(nil), baseArgs...), "--api-server=https://kubernetes.default.svc#credential"),
			wantError: "--api-server must be a valid HTTPS URL",
		},
		{
			name:      "legacy kubeconfig flag is rejected",
			args:      append(append([]string(nil), baseArgs...), "--kubeconfig=/tmp/admin.conf"),
			wantError: "flag provided but not defined: -kubeconfig",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := Run(context.Background(), test.args)
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("Run() error = %v, want %q", err, test.wantError)
			}
		})
	}
}
