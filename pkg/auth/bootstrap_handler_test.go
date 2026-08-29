package auth

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/realmroot/lightkite/pkg/common"
	"github.com/realmroot/lightkite/pkg/model"
)

func TestBootstrapAuthPublishesOneConfiguredOIDCProvider(t *testing.T) {
	original := common.OIDCProviderName
	common.OIDCProviderName = "Company Identity"
	t.Cleanup(func() { common.OIDCProviderName = original })

	value := (&AuthHandler{}).bootstrapAuth(&model.GeneralSetting{})
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

func TestBootstrapAuthUsesConfiguredLoginPrompt(t *testing.T) {
	value := (&AuthHandler{}).bootstrapAuth(&model.GeneralSetting{LoginPrompt: "  Sign in with your company account.  "})
	if value.LoginPrompt != "Sign in with your company account." {
		t.Fatalf("login prompt = %q", value.LoginPrompt)
	}
}

func TestBootstrapAuthUsesDefaultLoginPrompt(t *testing.T) {
	value := (&AuthHandler{}).bootstrapAuth(&model.GeneralSetting{})
	if value.LoginPrompt != "Kubernetes permissions come directly from your identity provider claims." {
		t.Fatalf("login prompt = %q", value.LoginPrompt)
	}
}
