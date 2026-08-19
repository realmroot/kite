package common

import (
	"os"
	"strings"
	"sync"
	"time"

	"k8s.io/klog/v2"
)

const (
	JWTExpirationSeconds = 24 * 60 * 60 // 24 hours
	DefaultJWTSecret     = "kite-default-jwt-secret-key-change-in-production"

	NodeTerminalPodName    = "kite-node-terminal-agent"
	KubectlTerminalPodName = "kite-kubectl-agent"

	KubectlAnnotation = "kubectl.kubernetes.io/last-applied-configuration"

	// db connection max idle time
	DBMaxIdleTime  = 10 * time.Minute
	DBMaxOpenConns = 100
	DBMaxIdleConns = 10
)

var (
	Port            = "8080"
	JwtSecret       = DefaultJWTSecret
	EnableAnalytics = false
	Host            = ""
	Base            = ""

	NodeTerminalImage    = "busybox:latest"
	KubectlTerminalImage = "zzde/kubectl:latest"
	ClusterAgentImage    = "ghcr.io/kite-org/kite:latest"
	OIDCIssuer           = ""
	OIDCClientID         = ""
	OIDCClientSecret     = ""
	OIDCProviderName     = "OpenID Connect"
	OIDCScopes           = []string{"openid", "profile", "email", "offline_access"}
	OIDCUsernameClaim    = "email"
	OIDCGroupsClaim      = "groups"
	OIDCNameClaim        = "name"
	OIDCPictureClaim     = "picture"
	PlatformAdminGroups  []string
	DBType               = "sqlite"
	DBDSN                = "dev.db"

	KiteEncryptKey = "kite-default-encryption-key-change-in-production"

	AllNamespaces = "_all"

	AnonymousUserEnabled = false

	CookieExpirationSeconds = 2 * JWTExpirationSeconds // double jwt

	DisableGZIP        = true
	EnableVersionCheck = true

	// CORSAllowedOrigins is empty by default (no CORS in production).
	// Developers can set CORS_ALLOWED_ORIGINS=http://localhost:5173 for
	// local Vite dev server cross-origin requests.
	CORSAllowedOrigins []string

	// TrustedProxies controls which direct peer IPs may provide forwarding headers.
	// Set TRUSTED_PROXIES to override the defaults, or TRUSTED_PROXIES=none to
	// ignore all client-supplied forwarding headers.
	TrustedProxies = []string{
		"127.0.0.0/8",
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"::1",
		"fc00::/7",
	}

	APIKeyProvider = "api_key"

	AgentPodNamespace = "kube-system"

	// ConfigFilePath is the path to the external config file (set via KITE_CONFIG_FILE env)
	ConfigFilePath = ""

	// ManagedSections tracks which configuration sections are managed by the config file.
	// Keys: "clusters", "oauth", "ldap", "rbac", "superUser"
	ManagedSections = map[string]bool{}
	managedMu       sync.RWMutex
)

const ManagedSectionError = "This section is managed by configuration file and cannot be modified through the UI"

func IsSectionManaged(section string) bool {
	managedMu.RLock()
	defer managedMu.RUnlock()
	return ManagedSections[section]
}

func SetManagedSections(sections map[string]bool) {
	managedMu.Lock()
	defer managedMu.Unlock()

	ManagedSections = make(map[string]bool, len(sections))
	for section, managed := range sections {
		if managed {
			ManagedSections[section] = true
		}
	}
}

func LoadEnvs() {
	if secret := os.Getenv("JWT_SECRET"); secret != "" {
		JwtSecret = secret
	}

	if port := os.Getenv("PORT"); port != "" {
		Port = port
	}

	if analytics := os.Getenv("ENABLE_ANALYTICS"); analytics == "true" {
		EnableAnalytics = true
	}
	if ns := os.Getenv("NAMESPACE"); ns != "" {
		AgentPodNamespace = ns
	}

	if nodeTerminalImage := os.Getenv("NODE_TERMINAL_IMAGE"); nodeTerminalImage != "" {
		NodeTerminalImage = nodeTerminalImage
	}

	if kubectlTerminalImage := os.Getenv("KUBECTL_TERMINAL_IMAGE"); kubectlTerminalImage != "" {
		KubectlTerminalImage = kubectlTerminalImage
	}

	if clusterAgentImage := os.Getenv("CLUSTER_AGENT_IMAGE"); clusterAgentImage != "" {
		ClusterAgentImage = clusterAgentImage
	}
	loadOIDCEnvs()
	loadDatabaseEnvs()

	if key := os.Getenv("KITE_ENCRYPT_KEY"); key != "" {
		KiteEncryptKey = key
	} else {
		klog.Warningf("KITE_ENCRYPT_KEY is not set, using default key, this is not secure for production!")
	}

	if v := os.Getenv("ANONYMOUS_USER_ENABLED"); v == "true" {
		AnonymousUserEnabled = true
		klog.Warningf("Anonymous user is enabled, this is not secure for production!")
	}
	if v := os.Getenv("HOST"); v != "" {
		Host = v
	}
	if v := os.Getenv("DISABLE_GZIP"); v != "" {
		DisableGZIP = v == "true"
	}

	if v := os.Getenv("DISABLE_VERSION_CHECK"); v == "true" {
		EnableVersionCheck = false
	}

	if v := os.Getenv("KITE_BASE"); v != "" {
		if v[0] != '/' {
			v = "/" + v
		}
		Base = strings.TrimRight(v, "/")
		klog.Infof("Using base path: %s", Base)
	}

	if v := os.Getenv("KITE_CONFIG_FILE"); v != "" {
		ConfigFilePath = v
	}

	if v := os.Getenv("CORS_ALLOWED_ORIGINS"); v != "" {
		origins := strings.Split(v, ",")
		for _, o := range origins {
			o = strings.TrimSpace(o)
			if o != "" {
				CORSAllowedOrigins = append(CORSAllowedOrigins, o)
			}
		}
		klog.Warningf("CORS enabled for origins: %v — disable in production", CORSAllowedOrigins)
	}

	if v := os.Getenv("TRUSTED_PROXIES"); v != "" {
		TrustedProxies = nil
		if !strings.EqualFold(strings.TrimSpace(v), "none") {
			proxies := strings.Split(v, ",")
			for _, proxy := range proxies {
				proxy = strings.TrimSpace(proxy)
				if proxy != "" {
					TrustedProxies = append(TrustedProxies, proxy)
				}
			}
		}
	}
	klog.Infof("Trusted proxies configured: %v", TrustedProxies)
}

func loadOIDCEnvs() {
	OIDCProviderName = "OpenID Connect"
	OIDCScopes = []string{"openid", "profile", "email", "offline_access"}
	OIDCUsernameClaim = "email"
	OIDCGroupsClaim = "groups"
	OIDCNameClaim = "name"
	OIDCPictureClaim = "picture"
	OIDCIssuer = strings.TrimRight(strings.TrimSpace(os.Getenv("OIDC_ISSUER")), "/")
	OIDCClientID = strings.TrimSpace(os.Getenv("OIDC_CLIENT_ID"))
	OIDCClientSecret = os.Getenv("OIDC_CLIENT_SECRET")
	if name := strings.TrimSpace(os.Getenv("OIDC_PROVIDER_NAME")); name != "" {
		OIDCProviderName = name
	}
	if scopes := splitConfigList(os.Getenv("OIDC_SCOPES")); len(scopes) != 0 {
		OIDCScopes = scopes
	}
	if claim := strings.TrimSpace(os.Getenv("OIDC_USERNAME_CLAIM")); claim != "" {
		OIDCUsernameClaim = claim
	}
	if claim := strings.TrimSpace(os.Getenv("OIDC_GROUPS_CLAIM")); claim != "" {
		OIDCGroupsClaim = claim
	}
	if claim := strings.TrimSpace(os.Getenv("OIDC_NAME_CLAIM")); claim != "" {
		OIDCNameClaim = claim
	}
	if claim := strings.TrimSpace(os.Getenv("OIDC_PICTURE_CLAIM")); claim != "" {
		OIDCPictureClaim = claim
	}
	PlatformAdminGroups = nil
	for _, group := range splitConfigList(os.Getenv("PLATFORM_ADMIN_GROUPS")) {
		if group = strings.TrimSpace(group); group != "" {
			PlatformAdminGroups = append(PlatformAdminGroups, group)
		}
	}
}

func splitConfigList(value string) []string {
	return strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t' || r == '\n'
	})
}

func loadDatabaseEnvs() {
	if dbDSN := os.Getenv("DB_DSN"); dbDSN != "" {
		DBDSN = dbDSN
	}
	if dbType := os.Getenv("DB_TYPE"); dbType != "" {
		if dbType != "sqlite" && dbType != "mysql" && dbType != "postgres" {
			klog.Fatalf("Invalid DB_TYPE: %s, must be one of sqlite, mysql, postgres", dbType)
		}
		DBType = dbType
	}
}
