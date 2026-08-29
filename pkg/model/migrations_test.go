package model

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/zxh326/kite/pkg/common"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestRemoveEmbeddedAIMigration(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := db.Exec(`CREATE TABLE general_settings (
		id integer primary key,
		ai_agent_enabled boolean,
		ai_provider text,
		ai_model text,
		ai_api_key text,
		ai_base_url text,
		ai_max_tokens integer,
		enable_analytics boolean
	)`).Error; err != nil {
		t.Fatalf("create legacy general_settings: %v", err)
	}
	if err := db.Exec(`CREATE TABLE pending_sessions (id integer primary key, session_id text)`).Error; err != nil {
		t.Fatalf("create legacy pending_sessions: %v", err)
	}
	if err := db.Exec(`CREATE TABLE resource_histories (id integer primary key, operation_source text)`).Error; err != nil {
		t.Fatalf("create resource_histories: %v", err)
	}
	if err := db.Exec(`INSERT INTO resource_histories (operation_source) VALUES ('ai'), ('manual')`).Error; err != nil {
		t.Fatalf("create resource history rows: %v", err)
	}

	if err := runSchemaMigrations(db); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	if db.Migrator().HasTable("pending_sessions") {
		t.Fatal("pending_sessions still exists")
	}
	var historySources []string
	if err := db.Table("resource_histories").Order("id").Pluck("operation_source", &historySources).Error; err != nil {
		t.Fatalf("read resource history rows: %v", err)
	}
	if len(historySources) != 2 || historySources[0] != "legacy" || historySources[1] != "manual" {
		t.Fatalf("resource history sources = %#v, want legacy and manual", historySources)
	}
	for _, column := range []string{
		"ai_agent_enabled", "ai_provider", "ai_model", "ai_api_key", "ai_base_url", "ai_max_tokens",
	} {
		if db.Migrator().HasColumn("general_settings", column) {
			t.Fatalf("legacy column %s still exists", column)
		}
	}
	if !db.Migrator().HasColumn("general_settings", "enable_analytics") {
		t.Fatal("unrelated general setting column was removed")
	}

	if err := runSchemaMigrations(db); err != nil {
		t.Fatalf("rerun migrations: %v", err)
	}
	var count int64
	if err := db.Model(&schemaMigration{}).Where("id = ?", "20260820_remove_embedded_ai").Count(&count).Error; err != nil {
		t.Fatalf("count migration records: %v", err)
	}
	if count != 1 {
		t.Fatalf("migration record count = %d, want 1", count)
	}
}

func TestEncodeOIDCGroupsAsJSONRevokesAmbiguousSessions(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`CREATE TABLE users (id integer primary key, oidc_groups text)`,
		`CREATE TABLE oidc_sessions (id integer primary key, user_id integer not null)`,
		`INSERT INTO users (id, oidc_groups) VALUES (1, 'developers,platform-admins')`,
		`INSERT INTO oidc_sessions (id, user_id) VALUES (10, 1)`,
	} {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatal(err)
		}
	}

	if err := encodeOIDCGroupsAsJSON(db); err != nil {
		t.Fatal(err)
	}
	var groups string
	if err := db.Table("users").Select("oidc_groups").Where("id = ?", 1).Scan(&groups).Error; err != nil {
		t.Fatal(err)
	}
	if groups != "[]" {
		t.Fatalf("OIDC groups = %q, want []", groups)
	}
	var sessionCount int64
	if err := db.Table("oidc_sessions").Count(&sessionCount).Error; err != nil {
		t.Fatal(err)
	}
	if sessionCount != 0 {
		t.Fatalf("session count = %d, want 0", sessionCount)
	}
}

func TestRemoveScheduledTasksDropsObsoleteAutomationState(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TABLE scheduled_tasks (id integer primary key, payload text)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := removeScheduledTasks(db); err != nil {
		t.Fatal(err)
	}
	if db.Migrator().HasTable("scheduled_tasks") {
		t.Fatal("obsolete scheduled_tasks table still exists")
	}
}

func TestRedactSecretResourceHistoryMigration(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TABLE resource_histories (
		id integer primary key, resource_type text, resource_yaml text, previous_yaml text, error_message text
	)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO resource_histories (id, resource_type, resource_yaml, previous_yaml, error_message) VALUES
		(1, 'secrets', 'current-secret', 'previous-secret', 'token must-not-leak'),
		(2, 'configmaps', 'current-config', 'previous-config', 'config error')`).Error; err != nil {
		t.Fatal(err)
	}

	if err := redactSecretResourceHistory(db); err != nil {
		t.Fatal(err)
	}
	var rows []struct {
		ID           uint
		ResourceYAML string
		PreviousYAML string
		ErrorMessage string
	}
	if err := db.Table("resource_histories").Order("id").Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].ResourceYAML != "" || rows[0].PreviousYAML != "" {
		t.Fatalf("secret row was not redacted: %#v", rows)
	}
	if rows[0].ErrorMessage != "Kubernetes Secret operation failed; details omitted" {
		t.Fatalf("secret error was not redacted: %#v", rows[0])
	}
	if rows[1].ResourceYAML != "current-config" || rows[1].PreviousYAML != "previous-config" {
		t.Fatalf("non-secret history changed: %#v", rows[1])
	}
	if rows[1].ErrorMessage != "config error" {
		t.Fatalf("non-secret error changed: %#v", rows[1])
	}
}

func TestNormalizeOIDCSessionTablePreservesOnlyConfiguredIssuer(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	statements := []string{
		`CREATE TABLE users (id integer primary key, issuer varchar(500) not null)`,
		`CREATE TABLE o_id_c_sessions (id integer primary key, user_id integer not null, token_hash text)`,
		`INSERT INTO users (id, issuer) VALUES (1, 'https://issuer.example'), (2, 'urn:kite:legacy')`,
		`INSERT INTO o_id_c_sessions (id, user_id, token_hash) VALUES (1, 1, 'current'), (2, 2, 'legacy')`,
	}
	for _, statement := range statements {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatal(err)
		}
	}
	previousIssuer := common.OIDCIssuer
	common.OIDCIssuer = "https://issuer.example"
	t.Cleanup(func() { common.OIDCIssuer = previousIssuer })

	if err := normalizeOIDCSessionTable(db); err != nil {
		t.Fatal(err)
	}
	if db.Migrator().HasTable("o_id_c_sessions") || !db.Migrator().HasTable("oidc_sessions") {
		t.Fatal("OIDC session table was not normalized")
	}
	var hashes []string
	if err := db.Table("oidc_sessions").Pluck("token_hash", &hashes).Error; err != nil {
		t.Fatal(err)
	}
	if len(hashes) != 1 || hashes[0] != "current" {
		t.Fatalf("remaining sessions = %#v", hashes)
	}
}

func TestReplaceUpstreamImageDefaultsPreservesOperatorOverrides(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TABLE general_settings (
		id integer primary key, kubectl_image text, node_terminal_image text,
		cluster_agent_image text
	)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO general_settings
		(id, kubectl_image, node_terminal_image, cluster_agent_image) VALUES
		(1, 'zzde/kubectl:latest', 'busybox:latest', 'ghcr.io/kite-org/kite:latest'),
		(2, 'operator/kubectl:v1', 'operator/node:v1', 'operator/agent:v1')`).Error; err != nil {
		t.Fatal(err)
	}
	previousAgentImage := common.ClusterAgentImage
	common.ClusterAgentImage = "registry.example.test/kite:v1.0.0"
	t.Cleanup(func() { common.ClusterAgentImage = previousAgentImage })

	if err := replaceUpstreamImageDefaults(db); err != nil {
		t.Fatal(err)
	}
	var rows []struct {
		ID                uint
		KubectlImage      string
		NodeTerminalImage string
		ClusterAgentImage string
	}
	if err := db.Table("general_settings").Order("id").Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("row count = %d", len(rows))
	}
	if rows[0].KubectlImage != DefaultGeneralKubectlImage ||
		rows[0].NodeTerminalImage != DefaultGeneralNodeTerminalImage ||
		rows[0].ClusterAgentImage != common.ClusterAgentImage {
		t.Fatalf("legacy defaults were not replaced: %#v", rows[0])
	}
	if rows[1].KubectlImage != "operator/kubectl:v1" ||
		rows[1].NodeTerminalImage != "operator/node:v1" ||
		rows[1].ClusterAgentImage != "operator/agent:v1" {
		t.Fatalf("operator overrides changed: %#v", rows[1])
	}
}

func TestReplaceShelllessKubectlImage(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TABLE general_settings (id integer primary key, kubectl_image text)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO general_settings (id, kubectl_image) VALUES
		(1, 'registry.k8s.io/kubectl:v1.36.3'), (2, 'operator/kubectl:v1')`).Error; err != nil {
		t.Fatal(err)
	}
	if err := replaceShelllessKubectlImage(db); err != nil {
		t.Fatal(err)
	}
	var images []string
	if err := db.Table("general_settings").Order("id").Pluck("kubectl_image", &images).Error; err != nil {
		t.Fatal(err)
	}
	if len(images) != 2 || images[0] != DefaultGeneralKubectlImage || images[1] != "operator/kubectl:v1" {
		t.Fatalf("kubectl images = %#v", images)
	}
}

func TestRemoveLegacyIdentityMigrationPreservesAttributionWithoutTrustEscalation(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := db.Exec("PRAGMA foreign_keys = ON").Error; err != nil {
		t.Fatalf("enable foreign keys: %v", err)
	}
	statements := []string{
		`CREATE TABLE users (
			id integer primary key, created_at datetime, updated_at datetime,
			username varchar(50) not null, password varchar(255), name varchar(100),
			avatar_url text, provider varchar(50), oidc_groups text, last_login_at datetime,
			enabled boolean, sub varchar(255), mfa_enabled boolean, mfa_secret text,
			api_key text, sidebar_preference text
		)`,
		`CREATE UNIQUE INDEX idx_users_username ON users(username)`,
		`CREATE TABLE oauth_providers (id integer primary key, name varchar(100), issuer varchar(255))`,
		`CREATE TABLE roles (id integer primary key, name text)`,
		`CREATE TABLE role_assignments (id integer primary key, role_id integer, subject text)`,
		`CREATE TABLE passkey_credentials (id integer primary key, user_id integer, credential text)`,
		`CREATE TABLE ldap_settings (id integer primary key, server_url text)`,
		`CREATE TABLE oidc_sessions (
			id integer primary key, token_hash varchar(64), user_id integer, id_token text,
			access_token text, refresh_token text, expires_at datetime
		)`,
		`CREATE TABLE resource_histories (
			id integer primary key, operator_id integer not null, operation_source text,
			FOREIGN KEY(operator_id) REFERENCES users(id) ON DELETE CASCADE
		)`,
		`INSERT INTO oauth_providers (id, name, issuer) VALUES
			(1, 'corp', 'https://identity.example.test/'),
			(2, 'oauth-without-issuer', '')`,
		`INSERT INTO users (id, username, password, provider, sub, enabled, mfa_secret, api_key)
			VALUES
			(1, 'oidc-user', 'password-hash', 'corp', 'subject-1', true, 'totp-secret', 'api-secret'),
			(2, 'local-user', 'password-hash', 'password', '', true, 'totp-secret', 'api-secret'),
			(3, 'untrusted-oauth-user', '', 'oauth-without-issuer', 'subject-3', true, '', '')`,
		`INSERT INTO oidc_sessions (id, token_hash, user_id, id_token, expires_at)
			VALUES (1, 'old-session', 1, 'old-id-token', '2099-01-01')`,
		`INSERT INTO resource_histories (id, operator_id, operation_source)
			VALUES (1, 1, 'manual')`,
	}
	for _, statement := range statements {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatalf("prepare legacy schema: %v", err)
		}
	}

	oldIssuer := common.OIDCIssuer
	common.OIDCIssuer = "https://new-issuer.example.test"
	t.Cleanup(func() { common.OIDCIssuer = oldIssuer })

	if err := runSchemaMigrations(db); err != nil {
		t.Fatalf("run migrations: %v", err)
	}

	for _, table := range []string{"oauth_providers", "roles", "role_assignments", "passkey_credentials", "ldap_settings"} {
		if db.Migrator().HasTable(table) {
			t.Errorf("legacy table %s still exists", table)
		}
	}
	for _, column := range []string{"password", "provider", "enabled", "mfa_enabled", "mfa_secret", "api_key"} {
		if db.Migrator().HasColumn("users", column) {
			t.Errorf("legacy users.%s still exists", column)
		}
	}

	var principals []struct {
		ID       uint
		Issuer   string
		Sub      string
		Username string
	}
	if err := db.Table("users").Order("id").Find(&principals).Error; err != nil {
		t.Fatalf("load migrated principals: %v", err)
	}
	want := []struct {
		issuer string
		sub    string
	}{
		{issuer: "https://identity.example.test", sub: "subject-1"},
		{issuer: "urn:kite:legacy", sub: "legacy:2"},
		{issuer: "urn:kite:legacy", sub: "legacy:3"},
	}
	if len(principals) != len(want) {
		t.Fatalf("principal count = %d, want %d", len(principals), len(want))
	}
	for index, principal := range principals {
		if principal.Username == "" {
			t.Errorf("principal %d lost its username during migration", principal.ID)
		}
		if principal.Issuer != want[index].issuer || principal.Sub != want[index].sub {
			t.Errorf("principal %d = (%q, %q), want (%q, %q)", principal.ID, principal.Issuer, principal.Sub, want[index].issuer, want[index].sub)
		}
	}

	var sessionCount int64
	if err := db.Table("oidc_sessions").Count(&sessionCount).Error; err != nil {
		t.Fatalf("count sessions: %v", err)
	}
	if sessionCount != 0 {
		t.Fatalf("legacy session count = %d, want 0", sessionCount)
	}
	var historyCount int64
	if err := db.Table("resource_histories").Count(&historyCount).Error; err != nil {
		t.Fatalf("count resource histories: %v", err)
	}
	if historyCount != 1 {
		t.Fatalf("resource history count = %d, want 1", historyCount)
	}
	var foreignKeyViolations int64
	if err := db.Raw("SELECT count(*) FROM pragma_foreign_key_check").Scan(&foreignKeyViolations).Error; err != nil {
		t.Fatalf("check foreign keys: %v", err)
	}
	if foreignKeyViolations != 0 {
		t.Fatalf("foreign key violations = %d, want 0", foreignKeyViolations)
	}

	// Prove that the migrated legacy schema converges to the current model,
	// including the non-unique username and composite issuer/subject indexes.
	if err := db.AutoMigrate(&User{}); err != nil {
		t.Fatalf("auto-migrate current user model: %v", err)
	}
	if !db.Migrator().HasIndex(&User{}, "idx_users_oidc_principal") {
		t.Fatal("current OIDC principal index was not created")
	}
	var usernameIndexUnique int
	if err := db.Raw(`SELECT "unique" FROM pragma_index_list('users') WHERE name = ?`, "idx_users_username").Row().Scan(&usernameIndexUnique); err != nil {
		t.Fatalf("inspect username index: %v", err)
	}
	if usernameIndexUnique != 0 {
		t.Fatal("username index is still unique")
	}

	if err := runSchemaMigrations(db); err != nil {
		t.Fatalf("rerun migrations: %v", err)
	}
}

func TestRemoveClusterCredentialsMigrationKeepsOnlyConnectionMetadata(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := db.Exec(`CREATE TABLE clusters (
		id integer primary key, created_at datetime, updated_at datetime,
		name varchar(100) not null, description text, config text,
		prometheus_url text, in_cluster boolean, cluster_agent boolean,
		is_default boolean, enable boolean
	)`).Error; err != nil {
		t.Fatalf("create legacy clusters: %v", err)
	}

	oldKey := common.KiteEncryptKey
	common.KiteEncryptKey = "migration-test-encryption-key"
	t.Cleanup(func() { common.KiteEncryptKey = oldKey })
	kubeconfig := `apiVersion: v1
kind: Config
current-context: production
clusters:
  - name: production
    cluster:
      server: https://api.example.test:6443
      certificate-authority-data: dGVzdC1jYQ==
      tls-server-name: api.internal.example.test
contexts:
  - name: production
    context:
      cluster: production
      user: privileged-user
users:
  - name: privileged-user
    user:
      token: cluster-admin-token-must-disappear
`
	statements := []struct {
		query string
		args  []any
	}{
		{query: `INSERT INTO clusters (id, name, config, in_cluster, cluster_agent, enable)
			VALUES (1, 'direct', ?, false, false, true)`, args: []any{SecretString(kubeconfig)}},
		{query: `INSERT INTO clusters (id, name, config, in_cluster, cluster_agent, enable)
			VALUES (2, 'old-in-cluster', '', true, false, true)`},
		{query: `INSERT INTO clusters (id, name, config, in_cluster, cluster_agent, enable)
			VALUES (3, 'tunnel', '', false, true, true)`},
	}
	for _, statement := range statements {
		if err := db.Exec(statement.query, statement.args...).Error; err != nil {
			t.Fatalf("seed legacy cluster: %v", err)
		}
	}

	if err := runSchemaMigrations(db); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	if db.Migrator().HasColumn("clusters", "config") {
		t.Fatal("credential-bearing clusters.config still exists")
	}
	if err := db.AutoMigrate(&Cluster{}); err != nil {
		t.Fatalf("auto-migrate current cluster model: %v", err)
	}

	var clusters []Cluster
	if err := db.Order("id").Find(&clusters).Error; err != nil {
		t.Fatalf("load migrated clusters: %v", err)
	}
	if len(clusters) != 3 {
		t.Fatalf("cluster count = %d, want 3", len(clusters))
	}
	direct := clusters[0]
	if direct.APIServerURL != "https://api.example.test:6443" ||
		direct.CABundle != "test-ca" ||
		direct.TLSServerName != "api.internal.example.test" ||
		direct.ConnectionMode != "direct" || !direct.Enable {
		t.Fatalf("direct cluster metadata = %#v", direct)
	}
	if clusters[1].Enable {
		t.Fatal("legacy in-cluster ServiceAccount connection was not disabled")
	}
	if clusters[2].ConnectionMode != "tunnel" || !clusters[2].Enable {
		t.Fatalf("tunnel cluster metadata = %#v", clusters[2])
	}

	var leakedCredentials int64
	if err := db.Raw(`SELECT count(*) FROM clusters
		WHERE api_server_url LIKE '%cluster-admin-token%'
		   OR ca_bundle LIKE '%cluster-admin-token%'
		   OR tls_server_name LIKE '%cluster-admin-token%'`).Scan(&leakedCredentials).Error; err != nil {
		t.Fatalf("scan migrated metadata for credentials: %v", err)
	}
	if leakedCredentials != 0 {
		t.Fatal("legacy Kubernetes credential leaked into connection metadata")
	}

	if err := runSchemaMigrations(db); err != nil {
		t.Fatalf("rerun migrations: %v", err)
	}
}

func TestKubeconfigMigrationRejectsCredentialsInAPIServerURL(t *testing.T) {
	value := `apiVersion: v1
kind: Config
current-context: production
clusters:
  - name: production
    cluster:
      server: https://admin:secret@api.example.test:6443
contexts:
  - name: production
    context:
      cluster: production
      user: legacy
users:
  - name: legacy
    user: {}
`
	if _, err := kubeconfigConnectionMetadata(value); err == nil {
		t.Fatal("credential-bearing API server URL was accepted as connection metadata")
	}
}

func TestBindResourceHistoryClustersMigration(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&Cluster{}); err != nil {
		t.Fatal(err)
	}
	cluster := Cluster{Name: "prod", ConnectionMode: "direct", APIServerURL: "https://api.example.test", Enable: true}
	if err := db.Create(&cluster).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TABLE resource_histories (
		id integer primary key, cluster_name varchar(100) not null,
		resource_type varchar(255) not null, resource_name varchar(255) not null,
		namespace varchar(100)
	)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO resource_histories
		(cluster_name, resource_type, resource_name, namespace)
		VALUES ('prod', 'configmaps', 'settings', 'default')`).Error; err != nil {
		t.Fatal(err)
	}
	if err := bindResourceHistoryClusters(db); err != nil {
		t.Fatal(err)
	}
	var clusterID uint
	if err := db.Table("resource_histories").Select("cluster_id").Where("id = ?", 1).Scan(&clusterID).Error; err != nil {
		t.Fatal(err)
	}
	if clusterID != cluster.ID {
		t.Fatalf("resource history cluster_id = %d, want %d", clusterID, cluster.ID)
	}
}
