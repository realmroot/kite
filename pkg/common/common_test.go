package common

import (
	"reflect"
	"testing"
)

func TestLoadEnvs(t *testing.T) {
	old := struct {
		Port                      string
		PprofAddress              string
		EnableAnalytics           bool
		AnalyticsScriptURL        string
		AnalyticsWebsiteID        string
		AgentPodNamespace         string
		NodeTerminalImage         string
		KubectlTerminalImage      string
		DBDSN                     string
		DBType                    string
		KiteEncryptKey            string
		Host                      string
		DisableGZIP               bool
		EnableVersionCheck        bool
		VersionCheckDisabledByEnv bool
		ReleaseAPIURL             string
		Base                      string
		CORSAllowedOrigins        []string
		TrustedProxies            []string
		OIDCIssuer                string
		OIDCCAFile                string
		OIDCClientID              string
		OIDCClientSecret          string
		OIDCProviderName          string
		OIDCScopes                []string
		OIDCUsernameClaim         string
		OIDCGroupsClaim           string
		PlatformAdminGroups       []string
		PlatformAdminSubjects     []string
	}{
		Port:                      Port,
		PprofAddress:              PprofAddress,
		EnableAnalytics:           EnableAnalytics,
		AnalyticsScriptURL:        AnalyticsScriptURL,
		AnalyticsWebsiteID:        AnalyticsWebsiteID,
		AgentPodNamespace:         AgentPodNamespace,
		NodeTerminalImage:         NodeTerminalImage,
		KubectlTerminalImage:      KubectlTerminalImage,
		DBDSN:                     DBDSN,
		DBType:                    DBType,
		KiteEncryptKey:            KiteEncryptKey,
		Host:                      Host,
		DisableGZIP:               DisableGZIP,
		EnableVersionCheck:        EnableVersionCheck,
		VersionCheckDisabledByEnv: VersionCheckDisabledByEnv,
		ReleaseAPIURL:             ReleaseAPIURL,
		Base:                      Base,
		CORSAllowedOrigins:        append([]string(nil), CORSAllowedOrigins...),
		TrustedProxies:            append([]string(nil), TrustedProxies...),
		OIDCIssuer:                OIDCIssuer,
		OIDCCAFile:                OIDCCAFile,
		OIDCClientID:              OIDCClientID,
		OIDCClientSecret:          OIDCClientSecret,
		OIDCProviderName:          OIDCProviderName,
		OIDCScopes:                append([]string(nil), OIDCScopes...),
		OIDCUsernameClaim:         OIDCUsernameClaim,
		OIDCGroupsClaim:           OIDCGroupsClaim,
		PlatformAdminGroups:       append([]string(nil), PlatformAdminGroups...),
		PlatformAdminSubjects:     append([]string(nil), PlatformAdminSubjects...),
	}
	defer func() {
		Port = old.Port
		PprofAddress = old.PprofAddress
		EnableAnalytics = old.EnableAnalytics
		AnalyticsScriptURL = old.AnalyticsScriptURL
		AnalyticsWebsiteID = old.AnalyticsWebsiteID
		AgentPodNamespace = old.AgentPodNamespace
		NodeTerminalImage = old.NodeTerminalImage
		KubectlTerminalImage = old.KubectlTerminalImage
		DBDSN = old.DBDSN
		DBType = old.DBType
		KiteEncryptKey = old.KiteEncryptKey
		Host = old.Host
		DisableGZIP = old.DisableGZIP
		EnableVersionCheck = old.EnableVersionCheck
		VersionCheckDisabledByEnv = old.VersionCheckDisabledByEnv
		ReleaseAPIURL = old.ReleaseAPIURL
		Base = old.Base
		CORSAllowedOrigins = append([]string(nil), old.CORSAllowedOrigins...)
		TrustedProxies = append([]string(nil), old.TrustedProxies...)
		OIDCIssuer = old.OIDCIssuer
		OIDCCAFile = old.OIDCCAFile
		OIDCClientID = old.OIDCClientID
		OIDCClientSecret = old.OIDCClientSecret
		OIDCProviderName = old.OIDCProviderName
		OIDCScopes = append([]string(nil), old.OIDCScopes...)
		OIDCUsernameClaim = old.OIDCUsernameClaim
		OIDCGroupsClaim = old.OIDCGroupsClaim
		PlatformAdminGroups = append([]string(nil), old.PlatformAdminGroups...)
		PlatformAdminSubjects = append([]string(nil), old.PlatformAdminSubjects...)
	}()

	CORSAllowedOrigins = nil
	TrustedProxies = nil

	t.Setenv("PORT", "9090")
	t.Setenv("PPROF_ADDRESS", "127.0.0.1:6060")
	t.Setenv("ENABLE_ANALYTICS", "true")
	t.Setenv("ANALYTICS_SCRIPT_URL", "https://analytics.example.test/script.js")
	t.Setenv("ANALYTICS_WEBSITE_ID", "kite-site")
	t.Setenv("NAMESPACE", "test-namespace")
	t.Setenv("NODE_TERMINAL_IMAGE", "test-node-image")
	t.Setenv("KUBECTL_TERMINAL_IMAGE", "test-kubectl-image")
	t.Setenv("DB_DSN", "test.db")
	t.Setenv("DB_TYPE", "mysql")
	t.Setenv("KITE_ENCRYPT_KEY", "test-encrypt-key")
	t.Setenv("HOST", "example.com")
	t.Setenv("DISABLE_GZIP", "false")
	t.Setenv("DISABLE_VERSION_CHECK", "true")
	t.Setenv("RELEASE_API_URL", "https://code.example.test/api/releases/latest")
	t.Setenv("KITE_BASE", "kite/")
	t.Setenv("CORS_ALLOWED_ORIGINS", "http://localhost:5173, https://example.com ,,")
	t.Setenv("TRUSTED_PROXIES", "10.42.0.0/16, 192.0.2.10 ,, ")
	t.Setenv("OIDC_ISSUER", "https://identity.example.com/")
	t.Setenv("OIDC_CA_FILE", "/etc/kite/identity-ca.pem")
	t.Setenv("OIDC_CLIENT_ID", "kite-client")
	t.Setenv("OIDC_CLIENT_SECRET", "client-secret")
	t.Setenv("OIDC_PROVIDER_NAME", "Company Login")
	t.Setenv("OIDC_SCOPES", "openid profile,email roles")
	t.Setenv("OIDC_USERNAME_CLAIM", "preferred_username")
	t.Setenv("OIDC_GROUPS_CLAIM", "roles")
	t.Setenv("PLATFORM_ADMIN_GROUPS", "operators, platform-admins")
	t.Setenv("PLATFORM_ADMIN_SUBJECTS", "subject-1,subject-2")

	LoadEnvs()

	if Port != "9090" {
		t.Fatalf("Port = %q, want %q", Port, "9090")
	}
	if PprofAddress != "127.0.0.1:6060" {
		t.Fatalf("PprofAddress = %q, want %q", PprofAddress, "127.0.0.1:6060")
	}
	if !EnableAnalytics {
		t.Fatalf("EnableAnalytics = %v, want true", EnableAnalytics)
	}
	assertAnalyticsConfigurationLoaded(t)
	if AgentPodNamespace != "test-namespace" {
		t.Fatalf("AgentPodNamespace = %q, want %q", AgentPodNamespace, "test-namespace")
	}
	if NodeTerminalImage != "test-node-image" {
		t.Fatalf("NodeTerminalImage = %q, want %q", NodeTerminalImage, "test-node-image")
	}
	if KubectlTerminalImage != "test-kubectl-image" {
		t.Fatalf("KubectlTerminalImage = %q, want %q", KubectlTerminalImage, "test-kubectl-image")
	}
	if DBDSN != "test.db" {
		t.Fatalf("DBDSN = %q, want %q", DBDSN, "test.db")
	}
	if DBType != "mysql" {
		t.Fatalf("DBType = %q, want %q", DBType, "mysql")
	}
	if KiteEncryptKey != "test-encrypt-key" {
		t.Fatalf("KiteEncryptKey = %q, want %q", KiteEncryptKey, "test-encrypt-key")
	}
	if Host != "example.com" {
		t.Fatalf("Host = %q, want %q", Host, "example.com")
	}
	if DisableGZIP {
		t.Fatalf("DisableGZIP = %v, want false", DisableGZIP)
	}
	if EnableVersionCheck {
		t.Fatalf("EnableVersionCheck = %v, want false", EnableVersionCheck)
	}
	if !VersionCheckDisabledByEnv {
		t.Fatal("VersionCheckDisabledByEnv = false, want true")
	}
	if ReleaseAPIURL != "https://code.example.test/api/releases/latest" {
		t.Fatalf("ReleaseAPIURL = %q", ReleaseAPIURL)
	}
	if Base != "/kite" {
		t.Fatalf("Base = %q, want %q", Base, "/kite")
	}

	wantOrigins := []string{"http://localhost:5173", "https://example.com"}
	if !reflect.DeepEqual(CORSAllowedOrigins, wantOrigins) {
		t.Fatalf("CORSAllowedOrigins = %#v, want %#v", CORSAllowedOrigins, wantOrigins)
	}

	wantTrustedProxies := []string{"10.42.0.0/16", "192.0.2.10"}
	if !reflect.DeepEqual(TrustedProxies, wantTrustedProxies) {
		t.Fatalf("TrustedProxies = %#v, want %#v", TrustedProxies, wantTrustedProxies)
	}
	if OIDCIssuer != "https://identity.example.com" || OIDCClientID != "kite-client" || OIDCClientSecret != "client-secret" {
		t.Fatalf("OIDC client configuration was not loaded")
	}
	if OIDCCAFile != "/etc/kite/identity-ca.pem" {
		t.Fatalf("OIDCCAFile = %q", OIDCCAFile)
	}
	if OIDCProviderName != "Company Login" || OIDCUsernameClaim != "preferred_username" || OIDCGroupsClaim != "roles" {
		t.Fatalf("OIDC display or claim configuration was not loaded")
	}
	if !reflect.DeepEqual(OIDCScopes, []string{"openid", "profile", "email", "roles"}) {
		t.Fatalf("OIDCScopes = %#v", OIDCScopes)
	}
	if !reflect.DeepEqual(PlatformAdminGroups, []string{"operators", "platform-admins"}) {
		t.Fatalf("PlatformAdminGroups = %#v", PlatformAdminGroups)
	}
	if !reflect.DeepEqual(PlatformAdminSubjects, []string{"subject-1", "subject-2"}) {
		t.Fatalf("PlatformAdminSubjects = %#v", PlatformAdminSubjects)
	}
}

func TestSplitPrincipalListPreservesStandardClaimValues(t *testing.T) {
	got := splitPrincipalList(`["operators,west","platform admins","team/a"]`)
	want := []string{"operators,west", "platform admins", "team/a"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("splitPrincipalList() = %#v, want %#v", got, want)
	}

	legacy := splitPrincipalList("operators, platform-admins")
	if !reflect.DeepEqual(legacy, []string{"operators", "platform-admins"}) {
		t.Fatalf("legacy splitPrincipalList() = %#v", legacy)
	}
}

func assertAnalyticsConfigurationLoaded(t *testing.T) {
	t.Helper()
	if AnalyticsScriptURL != "https://analytics.example.test/script.js" || AnalyticsWebsiteID != "kite-site" || !AnalyticsConfigured() {
		t.Fatal("analytics configuration was not loaded")
	}
}

func TestLoadEnvs_BaseAlreadyHasLeadingSlash(t *testing.T) {
	old := struct {
		KiteEncryptKey     string
		Base               string
		CORSAllowedOrigins []string
	}{
		KiteEncryptKey:     KiteEncryptKey,
		Base:               Base,
		CORSAllowedOrigins: append([]string(nil), CORSAllowedOrigins...),
	}
	defer func() {
		KiteEncryptKey = old.KiteEncryptKey
		Base = old.Base
		CORSAllowedOrigins = append([]string(nil), old.CORSAllowedOrigins...)
	}()

	t.Setenv("KITE_ENCRYPT_KEY", "test-encrypt-key")
	t.Setenv("KITE_BASE", "/kite/")

	LoadEnvs()

	if Base != "/kite" {
		t.Fatalf("Base = %q, want %q", Base, "/kite")
	}
}
