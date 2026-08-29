package utils

import (
	"strings"
	"testing"

	"github.com/realmroot/lightkite/pkg/common"
	"k8s.io/apimachinery/pkg/util/validation"
)

func TestInjectLightkiteBase(t *testing.T) {
	html := `<html><head><link rel="modulepreload" href="__KITE_BASE__/assets/index.js"><script type="module" src="__KITE_BASE__/assets/main.js"></script></head></html>`

	t.Run("subpath", func(t *testing.T) {
		got := InjectLightkiteBase(html, "/kite")

		if strings.Contains(got, "__KITE_BASE__") {
			t.Fatalf("placeholder should be replaced: %s", got)
		}
		if strings.Contains(got, "<base ") {
			t.Fatalf("base tag should not be injected anymore: %s", got)
		}
		if !strings.Contains(got, `href="/kite/assets/index.js"`) {
			t.Fatalf("expected asset href to include subpath: %s", got)
		}
		if !strings.Contains(got, `src="/kite/assets/main.js"`) {
			t.Fatalf("expected asset src to include subpath: %s", got)
		}
		if !strings.Contains(got, `<script>window.__dynamic_base__="/kite";</script>`) {
			t.Fatalf("expected runtime base script: %s", got)
		}
	})

	t.Run("root", func(t *testing.T) {
		got := InjectLightkiteBase(html, "")

		if !strings.Contains(got, `href="/assets/index.js"`) {
			t.Fatalf("expected root asset href: %s", got)
		}
		if !strings.Contains(got, `src="/assets/main.js"`) {
			t.Fatalf("expected root asset src: %s", got)
		}
		if !strings.Contains(got, `<script>window.__dynamic_base__="";</script>`) {
			t.Fatalf("expected empty runtime base script: %s", got)
		}
	})

	t.Run("escapes html attribute injection", func(t *testing.T) {
		got := InjectLightkiteBase(html, `/ki"te`)

		if strings.Contains(got, `href="/ki"te/assets/index.js"`) {
			t.Fatalf("expected asset href to be escaped: %s", got)
		}
		if !strings.Contains(got, `href="/ki&#34;te/assets/index.js"`) {
			t.Fatalf("expected escaped quote in asset href: %s", got)
		}
		if !strings.Contains(got, `<script>window.__dynamic_base__="/ki\"te";</script>`) {
			t.Fatalf("expected runtime base script to remain safely quoted: %s", got)
		}
	})
}

func TestInjectAnalytics(t *testing.T) {
	html := `<html><head><title>kite</title></head><body></body></html>`

	got := InjectAnalytics(html, `https://analytics.example.com/script.js?a="unsafe`, `site"><unsafe`)
	if !strings.Contains(got, `https://analytics.example.com/script.js?a=&#34;unsafe`) {
		t.Fatalf("expected analytics script to be injected: %s", got)
	}
	if !strings.Contains(got, `data-website-id="site&#34;&gt;&lt;unsafe"`) {
		t.Fatalf("expected analytics attributes to be escaped: %s", got)
	}
	if strings.Index(got, `analytics.example.com/script.js`) > strings.Index(got, `</head>`) {
		t.Fatalf("expected analytics script before </head>: %s", got)
	}

	if unchanged := InjectAnalytics("<html><body></body></html>", "https://analytics.example.com/script.js", "site"); unchanged != "<html><body></body></html>" {
		t.Fatalf("InjectAnalytics() = %q, want unchanged input", unchanged)
	}
}

func TestGenerateNodeAgentName(t *testing.T) {
	testcase := []struct {
		nodeName string
	}{
		{"node1"},
		{"shortname"},
		{"a-very-long-node-name-that-exceeds-the-maximum-length-allowed-for-kubernetes-names"},
		{"node-with-63-characters-abcdefghijklmnopqrstuvwxyz-123456789101"},
		{"ip-10-0-10-10.ch-west-2.compute.internal"},
		{"ip-10-0-10-10.ch-west-2.compute-internal"},
	}

	for _, tc := range testcase {
		podName := GenerateNodeAgentName(tc.nodeName)
		if errs := validation.IsDNS1123Subdomain(podName); len(errs) > 0 {
			t.Errorf("GenerateNodeAgentName(%q) = %q, invalid DNS subdomain: %v", tc.nodeName, podName, errs)
		}
	}
}

func TestGenerateKubectlAgentName(t *testing.T) {
	testCases := []struct {
		name     string
		username string
		prefix   string
	}{
		{
			name:     "sanitizes username",
			username: "Alice/Dev",
			prefix:   common.KubectlTerminalPodName + "-alice-dev-",
		},
		{
			name:     "falls back to user",
			username: "!!!",
			prefix:   common.KubectlTerminalPodName + "-user-",
		},
		{
			name:     "truncates long username",
			username: strings.Repeat("a", 80),
			prefix:   common.KubectlTerminalPodName + "-",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := GenerateKubectlAgentName(tc.username)
			if !strings.HasPrefix(got, tc.prefix) {
				t.Fatalf("GenerateKubectlAgentName(%q) = %q, want prefix %q", tc.username, got, tc.prefix)
			}
			if len(got) > 63 {
				t.Fatalf("GenerateKubectlAgentName(%q) = %q, want length <= 63", tc.username, got)
			}
			if errs := validation.IsDNS1123Subdomain(got); len(errs) > 0 {
				t.Fatalf("GenerateKubectlAgentName(%q) = %q, invalid DNS subdomain: %v", tc.username, got, errs)
			}
		})
	}
}
