package utils

import (
	"fmt"
	"html"
	"regexp"
	"strings"

	"k8s.io/apimachinery/pkg/util/rand"
)

const lightkiteBasePlaceholder = "__KITE_BASE__"

func InjectAnalytics(htmlContent, scriptURL, websiteID string) string {
	analyticsScript := fmt.Sprintf(
		`<script defer src="%s" data-website-id="%s" data-exclude-search="true" data-exclude-hash="true" data-do-not-track="true"></script>`,
		html.EscapeString(scriptURL),
		html.EscapeString(websiteID),
	)

	re := regexp.MustCompile(`</head>`)
	return re.ReplaceAllString(htmlContent, "  "+analyticsScript+"\n  </head>")
}

func InjectLightkiteBase(htmlContent string, base string) string {
	assetBase := base
	if assetBase == "/" {
		assetBase = ""
	}

	htmlContent = strings.ReplaceAll(htmlContent, lightkiteBasePlaceholder, html.EscapeString(assetBase))

	baseScript := fmt.Sprintf(`<script>window.__dynamic_base__=%q;</script>`, assetBase)
	re := regexp.MustCompile(`<head>`)
	return re.ReplaceAllString(htmlContent, "<head>\n    "+baseScript)
}

func RandomString(length int) string {
	return rand.String(length)
}
