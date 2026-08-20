package model

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/zxh326/kite/pkg/common"
	"gorm.io/gorm"
	"k8s.io/client-go/tools/clientcmd"
)

type schemaMigration struct {
	ID        string    `gorm:"primaryKey;type:varchar(100)"`
	AppliedAt time.Time `gorm:"not null"`
}

type migration struct {
	id                 string
	run                func(*gorm.DB) error
	outsideTransaction bool
}

var schemaMigrations = []migration{
	{id: "20260820_remove_embedded_ai", run: removeEmbeddedAI},
	{id: "20260820_remove_legacy_identity", run: removeLegacyIdentity, outsideTransaction: true},
	{id: "20260820_remove_cluster_credentials", run: removeClusterCredentials},
	{id: "20260820_normalize_oidc_session_table", run: normalizeOIDCSessionTable},
	{id: "20260820_replace_upstream_image_defaults", run: replaceUpstreamImageDefaults},
	{id: "20260820_replace_shellless_kubectl_image", run: replaceShelllessKubectlImage},
	{id: "20260820_redact_secret_resource_history", run: redactSecretResourceHistory},
	{id: "20260820_rebind_scheduled_task_actors", run: rebindScheduledTaskActors},
	{id: "20260820_bind_resource_history_clusters", run: bindResourceHistoryClusters},
	{id: "20260820_encode_oidc_groups_as_json", run: encodeOIDCGroupsAsJSON},
}

func encodeOIDCGroupsAsJSON(db *gorm.DB) error {
	migrator := db.Migrator()
	if !migrator.HasTable("users") || !migrator.HasColumn("users", "oidc_groups") {
		return nil
	}

	// The legacy comma-separated representation is ambiguous because an OIDC
	// group name may itself contain a comma. Do not reinterpret those values at
	// an authorization boundary: clear them and require a freshly verified token.
	if err := db.Table("users").Where("1 = 1").Update("oidc_groups", "[]").Error; err != nil {
		return fmt.Errorf("clear ambiguous legacy OIDC groups: %w", err)
	}
	if migrator.HasTable("scheduled_tasks") &&
		migrator.HasColumn("scheduled_tasks", "oidc_session_id") &&
		migrator.HasColumn("scheduled_tasks", "enabled") {
		updates := map[string]any{"enabled": false}
		if migrator.HasColumn("scheduled_tasks", "next_run_at") {
			updates["next_run_at"] = nil
		}
		if migrator.HasColumn("scheduled_tasks", "last_error") {
			updates["last_error"] = "OIDC group encoding upgraded; sign in and re-enable this task"
		}
		if err := db.Table("scheduled_tasks").
			Where("oidc_session_id <> ? AND enabled = ?", 0, true).
			Updates(updates).Error; err != nil {
			return fmt.Errorf("disable tasks bound to legacy OIDC sessions: %w", err)
		}
	}
	if migrator.HasTable("oidc_sessions") {
		if err := db.Exec("DELETE FROM oidc_sessions").Error; err != nil {
			return fmt.Errorf("revoke sessions with ambiguous legacy OIDC groups: %w", err)
		}
	}
	return nil
}

func bindResourceHistoryClusters(db *gorm.DB) error {
	migrator := db.Migrator()
	if !migrator.HasTable("resource_histories") || !migrator.HasTable("clusters") {
		return nil
	}
	if !migrator.HasColumn("resource_histories", "cluster_id") {
		if err := migrator.AddColumn(&ResourceHistory{}, "ClusterID"); err != nil {
			return fmt.Errorf("add resource history cluster identity: %w", err)
		}
	}
	var clusters []Cluster
	if err := db.Unscoped().Select("id", "name").Find(&clusters).Error; err != nil {
		return fmt.Errorf("list clusters for resource history binding: %w", err)
	}
	for _, cluster := range clusters {
		if err := db.Table("resource_histories").
			Where("(cluster_id IS NULL OR cluster_id = ?) AND cluster_name = ?", 0, cluster.Name).
			Update("cluster_id", cluster.ID).Error; err != nil {
			return fmt.Errorf("bind resource history for cluster %q: %w", cluster.Name, err)
		}
	}
	if err := db.Table("resource_histories").Where("cluster_id IS NULL").Update("cluster_id", 0).Error; err != nil {
		return fmt.Errorf("normalize unbound resource history cluster identity: %w", err)
	}
	return nil
}

func rebindScheduledTaskActors(db *gorm.DB) error {
	migrator := db.Migrator()
	if !migrator.HasTable("scheduled_tasks") || !migrator.HasTable("oidc_sessions") ||
		!migrator.HasColumn("scheduled_tasks", "creator_id") ||
		!migrator.HasColumn("scheduled_tasks", "oidc_session_id") ||
		!migrator.HasColumn("oidc_sessions", "user_id") {
		return nil
	}
	if err := db.Exec(`UPDATE scheduled_tasks
		SET creator_id = (
			SELECT oidc_sessions.user_id FROM oidc_sessions
			WHERE oidc_sessions.id = scheduled_tasks.oidc_session_id
		)
		WHERE oidc_session_id <> 0 AND EXISTS (
			SELECT 1 FROM oidc_sessions
			WHERE oidc_sessions.id = scheduled_tasks.oidc_session_id
		)`).Error; err != nil {
		return fmt.Errorf("rebind scheduled task actors to OIDC sessions: %w", err)
	}
	return nil
}

func redactSecretResourceHistory(db *gorm.DB) error {
	migrator := db.Migrator()
	if !migrator.HasTable("resource_histories") ||
		!migrator.HasColumn("resource_histories", "resource_type") ||
		!migrator.HasColumn("resource_histories", "resource_yaml") ||
		!migrator.HasColumn("resource_histories", "previous_yaml") {
		return nil
	}
	updates := map[string]any{
		"resource_yaml": "",
		"previous_yaml": "",
	}
	if migrator.HasColumn("resource_histories", "error_message") {
		updates["error_message"] = gorm.Expr("CASE WHEN error_message = '' THEN '' ELSE ? END", "Kubernetes Secret operation failed; details omitted")
	}
	if err := db.Table("resource_histories").Where("resource_type = ?", "secrets").Updates(updates).Error; err != nil {
		return fmt.Errorf("redact Kubernetes Secret history: %w", err)
	}
	return nil
}

func runSchemaMigrations(db *gorm.DB) error {
	if err := db.AutoMigrate(&schemaMigration{}); err != nil {
		return fmt.Errorf("create schema migrations table: %w", err)
	}
	for _, item := range schemaMigrations {
		var applied int64
		if err := db.Model(&schemaMigration{}).Where("id = ?", item.id).Count(&applied).Error; err != nil {
			return fmt.Errorf("check migration %s: %w", item.id, err)
		}
		if applied != 0 {
			continue
		}
		apply := func(target *gorm.DB) error {
			if err := item.run(target); err != nil {
				return err
			}
			return target.Create(&schemaMigration{ID: item.id, AppliedAt: time.Now().UTC()}).Error
		}
		var err error
		if item.outsideTransaction {
			err = apply(db)
		} else {
			err = db.Transaction(apply)
		}
		if err != nil {
			return fmt.Errorf("apply migration %s: %w", item.id, err)
		}
	}
	return nil
}

func removeEmbeddedAI(db *gorm.DB) error {
	migrator := db.Migrator()
	if migrator.HasTable("resource_histories") && migrator.HasColumn("resource_histories", "operation_source") {
		if err := db.Exec("UPDATE resource_histories SET operation_source = ? WHERE operation_source = ?", "legacy", "ai").Error; err != nil {
			return fmt.Errorf("normalize embedded AI audit records: %w", err)
		}
	}
	if migrator.HasTable("pending_sessions") {
		if err := migrator.DropTable("pending_sessions"); err != nil {
			return fmt.Errorf("drop pending_sessions: %w", err)
		}
	}
	if !migrator.HasTable(&GeneralSetting{}) {
		return nil
	}
	for _, column := range []string{
		"ai_agent_enabled",
		"ai_provider",
		"ai_model",
		"ai_api_key",
		"ai_base_url",
		"ai_max_tokens",
	} {
		if !migrator.HasColumn("general_settings", column) {
			continue
		}
		if err := db.Exec("ALTER TABLE general_settings DROP COLUMN " + column).Error; err != nil {
			return fmt.Errorf("drop general_settings.%s: %w", column, err)
		}
	}
	return nil
}

func removeLegacyIdentity(db *gorm.DB) error {
	if db.Name() != "sqlite" {
		return db.Transaction(removeLegacyIdentitySchema)
	}

	// SQLite cannot alter column widths or NOT NULL constraints in place. Pin a
	// single connection, suspend FK enforcement only for the schema transaction,
	// and validate every reference before returning it to the pool.
	return db.Connection(func(conn *gorm.DB) error {
		var foreignKeysEnabled int
		if err := conn.Raw("PRAGMA foreign_keys").Scan(&foreignKeysEnabled).Error; err != nil {
			return fmt.Errorf("read SQLite foreign key mode: %w", err)
		}
		if foreignKeysEnabled != 0 {
			if err := conn.Exec("PRAGMA foreign_keys = OFF").Error; err != nil {
				return fmt.Errorf("disable SQLite foreign keys for identity migration: %w", err)
			}
		}
		err := conn.Transaction(removeLegacyIdentitySchema)
		if foreignKeysEnabled != 0 {
			if enableErr := conn.Exec("PRAGMA foreign_keys = ON").Error; enableErr != nil && err == nil {
				err = fmt.Errorf("restore SQLite foreign key enforcement: %w", enableErr)
			}
		}
		if err != nil {
			return err
		}
		var violationCount int64
		if err := conn.Raw("SELECT count(*) FROM pragma_foreign_key_check").Scan(&violationCount).Error; err != nil {
			return fmt.Errorf("validate SQLite foreign keys: %w", err)
		}
		if violationCount != 0 {
			return fmt.Errorf("identity migration left %d SQLite foreign key violation(s)", violationCount)
		}
		return nil
	})
}

//nolint:gocyclo // This is one atomic, dialect-aware legacy schema migration.
func removeLegacyIdentitySchema(db *gorm.DB) error {
	migrator := db.Migrator()
	legacyIdentityPresent := false
	for _, table := range []string{
		"role_assignments", "roles", "passkey_credentials", "ldap_settings", "oauth_providers",
	} {
		if migrator.HasTable(table) {
			legacyIdentityPresent = true
		}
	}

	if migrator.HasTable("general_settings") {
		for _, column := range []string{"password_login_disabled", "enable_mfa", "enable_passkey_login"} {
			if migrator.HasColumn("general_settings", column) {
				if err := db.Exec("ALTER TABLE general_settings DROP COLUMN " + column).Error; err != nil {
					return fmt.Errorf("drop general_settings.%s: %w", column, err)
				}
			}
		}
	}

	if !migrator.HasTable("users") {
		return dropLegacyIdentityTables(db)
	}
	for _, column := range []string{"password", "provider", "enabled", "mfa_enabled", "mfa_secret", "api_key"} {
		if migrator.HasColumn("users", column) {
			legacyIdentityPresent = true
		}
	}

	providerIssuers := map[string]string{}
	if migrator.HasTable("oauth_providers") &&
		migrator.HasColumn("oauth_providers", "name") &&
		migrator.HasColumn("oauth_providers", "issuer") {
		rows, err := db.Table("oauth_providers").Select([]string{"name", "issuer"}).Rows()
		if err != nil {
			return fmt.Errorf("load legacy OIDC providers: %w", err)
		}
		for rows.Next() {
			var name, rawIssuer string
			if err := rows.Scan(&name, &rawIssuer); err != nil {
				_ = rows.Close()
				return fmt.Errorf("scan legacy OIDC provider: %w", err)
			}
			issuer := strings.TrimRight(strings.TrimSpace(rawIssuer), "/")
			if issuer != "" {
				providerIssuers[strings.ToLower(strings.TrimSpace(name))] = issuer
			}
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return fmt.Errorf("iterate legacy OIDC providers: %w", err)
		}
		if err := rows.Close(); err != nil {
			return fmt.Errorf("close legacy OIDC provider rows: %w", err)
		}
	}

	if !migrator.HasColumn("users", "issuer") {
		if err := db.Exec("ALTER TABLE users ADD COLUMN issuer varchar(500)").Error; err != nil {
			return fmt.Errorf("add users.issuer: %w", err)
		}
	}
	var principals []struct {
		ID       uint
		Sub      string
		Provider string
	}
	principalColumns := []string{"id", "sub"}
	hasProviderColumn := migrator.HasColumn("users", "provider")
	if hasProviderColumn {
		principalColumns = append(principalColumns, "provider")
	}
	rows, err := db.Table("users").Select(principalColumns).Rows()
	if err != nil {
		return fmt.Errorf("load legacy users: %w", err)
	}
	for rows.Next() {
		var principal struct {
			ID       uint
			Sub      string
			Provider string
		}
		var scanErr error
		if hasProviderColumn {
			scanErr = rows.Scan(&principal.ID, &principal.Sub, &principal.Provider)
		} else {
			scanErr = rows.Scan(&principal.ID, &principal.Sub)
		}
		if scanErr != nil {
			_ = rows.Close()
			return fmt.Errorf("scan legacy user: %w", scanErr)
		}
		principals = append(principals, principal)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("iterate legacy users: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close legacy user rows: %w", err)
	}
	for _, principal := range principals {
		issuer := ""
		subject := principal.Sub
		if providerIssuer := providerIssuers[strings.ToLower(strings.TrimSpace(principal.Provider))]; providerIssuer != "" && subject != "" {
			issuer = providerIssuer
		} else if !legacyIdentityPresent && common.OIDCIssuer != "" && subject != "" {
			// This is an already OIDC-only profile from an intermediate release.
			issuer = common.OIDCIssuer
		} else {
			// Local accounts and users from providers without a verifiable issuer
			// remain available for historical attribution, but can never become a
			// principal trusted by the configured issuer accidentally.
			issuer = "urn:kite:legacy"
			subject = fmt.Sprintf("legacy:%d", principal.ID)
		}
		if err := db.Table("users").Where("id = ?", principal.ID).Updates(map[string]interface{}{
			"issuer": issuer,
			"sub":    subject,
		}).Error; err != nil {
			return fmt.Errorf("migrate legacy user %d: %w", principal.ID, err)
		}
	}
	if legacyIdentityPresent && migrator.HasTable("oidc_sessions") {
		if err := db.Exec("DELETE FROM oidc_sessions").Error; err != nil {
			return fmt.Errorf("revoke sessions from legacy identity providers: %w", err)
		}
	}
	if db.Name() != "sqlite" {
		for _, index := range []string{"idx_users_provider", "idx_users_enabled", "idx_users_api_key"} {
			if migrator.HasIndex("users", index) {
				if err := migrator.DropIndex("users", index); err != nil {
					return fmt.Errorf("drop legacy user index %s: %w", index, err)
				}
			}
		}
		for _, column := range []string{"password", "provider", "enabled", "mfa_enabled", "mfa_secret", "api_key"} {
			if migrator.HasColumn("users", column) {
				if err := db.Exec("ALTER TABLE users DROP COLUMN " + column).Error; err != nil {
					return fmt.Errorf("drop users.%s: %w", column, err)
				}
			}
		}
	}
	if db.Name() != "sqlite" && migrator.HasIndex("users", "idx_users_username") {
		if db.Name() == "mysql" {
			if err := db.Exec("ALTER TABLE users DROP INDEX idx_users_username").Error; err != nil {
				return fmt.Errorf("drop unique username index: %w", err)
			}
		} else if err := db.Exec("DROP INDEX idx_users_username").Error; err != nil {
			return fmt.Errorf("drop unique username index: %w", err)
		}
	}
	if legacyIdentityPresent {
		if err := convergeUserSchema(db); err != nil {
			return err
		}
	}
	return dropLegacyIdentityTables(db)
}

func convergeUserSchema(db *gorm.DB) error {
	switch db.Name() {
	case "sqlite":
		groupsColumn := "'[]'"
		if db.Migrator().HasColumn("users", "oidc_groups") {
			groupsColumn = "oidc_groups"
		} else if db.Migrator().HasColumn("users", "o_id_c_groups") {
			groupsColumn = "o_id_c_groups"
		}
		statements := []string{
			`CREATE TABLE users_oidc_migration (
				id integer primary key, created_at datetime, updated_at datetime,
				issuer varchar(500) NOT NULL, sub varchar(500) NOT NULL,
				username varchar(255) NOT NULL, name varchar(255), avatar_url text,
				oidc_groups text, last_login_at timestamp, sidebar_preference text
			)`,
			fmt.Sprintf(`INSERT INTO users_oidc_migration (
				id, created_at, updated_at, issuer, sub, username, name, avatar_url,
				oidc_groups, last_login_at, sidebar_preference
			) SELECT
				id, created_at, updated_at, issuer, sub, username, name, avatar_url,
				%s, last_login_at, sidebar_preference
			FROM users`, groupsColumn),
			`DROP TABLE users`,
			`ALTER TABLE users_oidc_migration RENAME TO users`,
		}
		for _, statement := range statements {
			if err := db.Exec(statement).Error; err != nil {
				return fmt.Errorf("converge SQLite users table: %w", err)
			}
		}
	case "mysql":
		if err := db.Exec(`ALTER TABLE users
			MODIFY issuer varchar(500) NOT NULL,
			MODIFY sub varchar(500) NOT NULL,
			MODIFY username varchar(255) NOT NULL,
			MODIFY name varchar(255) NULL`).Error; err != nil {
			return fmt.Errorf("converge MySQL users table: %w", err)
		}
	case "postgres":
		if err := db.Exec(`ALTER TABLE users
			ALTER COLUMN issuer TYPE varchar(500),
			ALTER COLUMN issuer SET NOT NULL,
			ALTER COLUMN sub TYPE varchar(500),
			ALTER COLUMN sub SET NOT NULL,
			ALTER COLUMN username TYPE varchar(255),
			ALTER COLUMN username SET NOT NULL,
			ALTER COLUMN name TYPE varchar(255)`).Error; err != nil {
			return fmt.Errorf("converge PostgreSQL users table: %w", err)
		}
	default:
		return fmt.Errorf("unsupported database dialect %q", db.Name())
	}
	return nil
}

func dropLegacyIdentityTables(db *gorm.DB) error {
	migrator := db.Migrator()
	for _, table := range []string{
		"role_assignments", "roles", "passkey_credentials", "ldap_settings", "oauth_providers",
	} {
		if !migrator.HasTable(table) {
			continue
		}
		if err := migrator.DropTable(table); err != nil {
			return fmt.Errorf("drop %s: %w", table, err)
		}
	}
	return nil
}

func normalizeOIDCSessionTable(db *gorm.DB) error {
	migrator := db.Migrator()
	const legacyTable = "o_id_c_sessions"
	const canonicalTable = "oidc_sessions"
	if migrator.HasTable(legacyTable) && migrator.HasTable(canonicalTable) {
		return fmt.Errorf("both %s and %s exist; refusing to guess which session store is authoritative", legacyTable, canonicalTable)
	}
	if migrator.HasTable(legacyTable) {
		if err := migrator.RenameTable(legacyTable, canonicalTable); err != nil {
			return fmt.Errorf("rename legacy OIDC session table: %w", err)
		}
	}
	if !migrator.HasTable(canonicalTable) || !migrator.HasTable("users") || !migrator.HasColumn("users", "issuer") {
		return nil
	}
	if err := db.Exec(`DELETE FROM oidc_sessions
		WHERE user_id IN (SELECT id FROM users WHERE issuer <> ?)`, common.OIDCIssuer).Error; err != nil {
		return fmt.Errorf("revoke sessions not issued by the configured OIDC provider: %w", err)
	}
	return nil
}

func replaceUpstreamImageDefaults(db *gorm.DB) error {
	if !db.Migrator().HasTable("general_settings") {
		return nil
	}
	updates := []struct {
		column string
		legacy string
		value  string
	}{
		{column: "kubectl_image", legacy: "zzde/kubectl:latest", value: DefaultGeneralKubectlImageValue()},
		{column: "node_terminal_image", legacy: "busybox:latest", value: DefaultGeneralNodeTerminalImageValue()},
		{column: "cluster_agent_image", legacy: "ghcr.io/kite-org/kite:latest", value: DefaultGeneralClusterAgentImageValue()},
	}
	for _, update := range updates {
		if !db.Migrator().HasColumn("general_settings", update.column) {
			continue
		}
		if err := db.Table("general_settings").Where(update.column+" = ?", update.legacy).Update(update.column, update.value).Error; err != nil {
			return fmt.Errorf("replace legacy general_settings.%s image: %w", update.column, err)
		}
	}
	return nil
}

func replaceShelllessKubectlImage(db *gorm.DB) error {
	if !db.Migrator().HasTable("general_settings") || !db.Migrator().HasColumn("general_settings", "kubectl_image") {
		return nil
	}
	if err := db.Table("general_settings").
		Where("kubectl_image = ?", "registry.k8s.io/kubectl:v1.36.3").
		Update("kubectl_image", DefaultGeneralKubectlImageValue()).Error; err != nil {
		return fmt.Errorf("replace shellless kubectl terminal image: %w", err)
	}
	return nil
}

func removeClusterCredentials(db *gorm.DB) error {
	migrator := db.Migrator()
	if !migrator.HasTable("clusters") || !migrator.HasColumn("clusters", "config") {
		return nil
	}
	hasInCluster := migrator.HasColumn("clusters", "in_cluster")
	hasClusterAgent := migrator.HasColumn("clusters", "cluster_agent")
	hasEnabled := migrator.HasColumn("clusters", "enable")

	columns := []struct {
		name       string
		definition string
	}{
		{name: "description", definition: "text"},
		{name: "api_server_url", definition: "text"},
		{name: "ca_bundle", definition: "text"},
		{name: "tls_server_name", definition: "varchar(255)"},
		{name: "connection_mode", definition: "varchar(20) DEFAULT 'direct'"},
		{name: "prometheus_url", definition: "text"},
		{name: "in_cluster", definition: "boolean DEFAULT false"},
		{name: "cluster_agent", definition: "boolean DEFAULT false"},
		{name: "cluster_agent_token_hash", definition: "varchar(64)"},
		{name: "cluster_agent_public_key", definition: "varchar(64)"},
		{name: "cluster_agent_private_key", definition: "text"},
		{name: "is_default", definition: "boolean DEFAULT false"},
		{name: "enable", definition: "boolean DEFAULT true"},
	}
	for _, column := range columns {
		if migrator.HasColumn("clusters", column.name) {
			continue
		}
		if err := db.Exec("ALTER TABLE clusters ADD COLUMN " + column.name + " " + column.definition).Error; err != nil {
			return fmt.Errorf("add clusters.%s: %w", column.name, err)
		}
	}

	type legacyCluster struct {
		ID           uint
		Config       SecretString
		InCluster    bool
		ClusterAgent bool
		Enabled      bool
	}
	selectColumns := []string{"id", "config"}
	if hasInCluster {
		selectColumns = append(selectColumns, "in_cluster")
	}
	if hasClusterAgent {
		selectColumns = append(selectColumns, "cluster_agent")
	}
	if hasEnabled {
		selectColumns = append(selectColumns, "enable")
	}
	rows, err := db.Table("clusters").Select(selectColumns).Rows()
	if err != nil {
		return fmt.Errorf("load clusters with legacy credentials: %w", err)
	}
	var clusters []legacyCluster
	for rows.Next() {
		var cluster legacyCluster
		destinations := []any{&cluster.ID, &cluster.Config}
		if hasInCluster {
			destinations = append(destinations, &cluster.InCluster)
		}
		if hasClusterAgent {
			destinations = append(destinations, &cluster.ClusterAgent)
		}
		if hasEnabled {
			destinations = append(destinations, &cluster.Enabled)
		} else {
			cluster.Enabled = true
		}
		if err := rows.Scan(destinations...); err != nil {
			_ = rows.Close()
			return fmt.Errorf("scan cluster with legacy credentials: %w", err)
		}
		clusters = append(clusters, cluster)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("iterate clusters with legacy credentials: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close legacy cluster rows: %w", err)
	}

	for _, cluster := range clusters {
		updates := map[string]any{
			"connection_mode": "direct",
			"enable":          cluster.Enabled,
		}
		switch {
		case cluster.ClusterAgent:
			updates["connection_mode"] = "tunnel"
		case strings.TrimSpace(string(cluster.Config)) != "":
			metadata, err := kubeconfigConnectionMetadata(string(cluster.Config))
			if err != nil {
				return fmt.Errorf("extract credential-free metadata for cluster %d: %w", cluster.ID, err)
			}
			updates["api_server_url"] = metadata.apiServerURL
			updates["ca_bundle"] = metadata.caBundle
			updates["tls_server_name"] = metadata.tlsServerName
		case cluster.InCluster:
			// The old in-cluster mode depended on Kite's mounted ServiceAccount.
			// Preserve the catalog row but require explicit direct/tunnel setup.
			updates["enable"] = false
		}
		if err := db.Table("clusters").Where("id = ?", cluster.ID).Updates(updates).Error; err != nil {
			return fmt.Errorf("migrate cluster %d connection metadata: %w", cluster.ID, err)
		}
	}

	// Overwrite before dropping so credentials are gone even on engines where
	// dropped-column storage is not reclaimed immediately.
	if err := db.Exec("UPDATE clusters SET config = ''").Error; err != nil {
		return fmt.Errorf("erase legacy cluster credentials: %w", err)
	}
	if err := db.Exec("ALTER TABLE clusters DROP COLUMN config").Error; err != nil {
		return fmt.Errorf("drop clusters.config: %w", err)
	}
	if err := convergeClusterSchema(db); err != nil {
		return err
	}
	return nil
}

func convergeClusterSchema(db *gorm.DB) error {
	switch db.Name() {
	case "sqlite":
		statements := []string{
			`CREATE TABLE clusters_oidc_migration (
				id integer primary key, created_at datetime, updated_at datetime,
				name varchar(100) NOT NULL, description text, api_server_url text,
				ca_bundle text, tls_server_name varchar(255),
				connection_mode varchar(20) DEFAULT 'direct', prometheus_url text,
				in_cluster boolean DEFAULT false, cluster_agent boolean DEFAULT false,
				cluster_agent_token_hash varchar(64), cluster_agent_public_key varchar(64),
				cluster_agent_private_key text, is_default boolean DEFAULT false,
				enable boolean DEFAULT true
			)`,
			`INSERT INTO clusters_oidc_migration (
				id, created_at, updated_at, name, description, api_server_url, ca_bundle,
				tls_server_name, connection_mode, prometheus_url, in_cluster,
				cluster_agent, cluster_agent_token_hash, cluster_agent_public_key,
				cluster_agent_private_key, is_default, enable
			) SELECT
				id, created_at, updated_at, name, description, api_server_url, ca_bundle,
				tls_server_name, connection_mode, prometheus_url, in_cluster,
				cluster_agent, cluster_agent_token_hash, cluster_agent_public_key,
				cluster_agent_private_key, is_default, enable
			FROM clusters`,
			`DROP TABLE clusters`,
			`ALTER TABLE clusters_oidc_migration RENAME TO clusters`,
		}
		for _, statement := range statements {
			if err := db.Exec(statement).Error; err != nil {
				return fmt.Errorf("converge SQLite clusters table: %w", err)
			}
		}
	case "mysql":
		if err := db.Exec(`ALTER TABLE clusters
			MODIFY connection_mode varchar(20) DEFAULT 'direct',
			MODIFY enable boolean DEFAULT true`).Error; err != nil {
			return fmt.Errorf("converge MySQL clusters table: %w", err)
		}
	case "postgres":
		if err := db.Exec(`ALTER TABLE clusters
			ALTER COLUMN connection_mode SET DEFAULT 'direct',
			ALTER COLUMN enable SET DEFAULT true`).Error; err != nil {
			return fmt.Errorf("converge PostgreSQL clusters table: %w", err)
		}
	default:
		return fmt.Errorf("unsupported database dialect %q", db.Name())
	}
	return nil
}

type clusterConnectionMetadata struct {
	apiServerURL  string
	caBundle      string
	tlsServerName string
}

func kubeconfigConnectionMetadata(value string) (*clusterConnectionMetadata, error) {
	config, err := clientcmd.Load([]byte(value))
	if err != nil {
		return nil, fmt.Errorf("parse kubeconfig: %w", err)
	}
	context := config.Contexts[config.CurrentContext]
	if context == nil && len(config.Contexts) == 1 {
		for _, candidate := range config.Contexts {
			context = candidate
		}
	}
	if context == nil || strings.TrimSpace(context.Cluster) == "" {
		return nil, errors.New("kubeconfig has no current cluster context")
	}
	cluster := config.Clusters[context.Cluster]
	if cluster == nil || strings.TrimSpace(cluster.Server) == "" {
		return nil, errors.New("kubeconfig current context has no API server")
	}
	apiServerURL, err := url.Parse(strings.TrimSpace(cluster.Server))
	if err != nil || apiServerURL.Scheme != "https" || apiServerURL.Host == "" || apiServerURL.User != nil || apiServerURL.RawQuery != "" || apiServerURL.ForceQuery || apiServerURL.Fragment != "" {
		return nil, errors.New("kubeconfig API server must be a credential-free HTTPS URL")
	}
	if cluster.InsecureSkipTLSVerify {
		return nil, errors.New("insecure-skip-tls-verify cannot be migrated")
	}
	if cluster.CertificateAuthority != "" && len(cluster.CertificateAuthorityData) == 0 {
		return nil, errors.New("external certificate-authority paths cannot be migrated")
	}
	return &clusterConnectionMetadata{
		apiServerURL:  strings.TrimSpace(cluster.Server),
		caBundle:      string(cluster.CertificateAuthorityData),
		tlsServerName: strings.TrimSpace(cluster.TLSServerName),
	}, nil
}
