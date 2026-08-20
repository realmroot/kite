package auth

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/zxh326/kite/pkg/common"
)

func TestBootstrapAuthPublishesOneConfiguredOIDCProvider(t *testing.T) {
	original := common.OIDCProviderName
	common.OIDCProviderName = "Company Identity"
	t.Cleanup(func() { common.OIDCProviderName = original })

	value := (&AuthHandler{}).bootstrapAuth()
	if value.ProviderName != "Company Identity" {
		t.Fatalf("provider name = %q", value.ProviderName)
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	text := string(encoded)
	for _, legacyField := range []string{`"providers"`, `"oauthProviders"`, `"setup"`, `"step"`} {
		if strings.Contains(text, legacyField) {
			t.Fatalf("bootstrap auth contains legacy field %s: %s", legacyField, text)
		}
	}
}
