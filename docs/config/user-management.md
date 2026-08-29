# Identity lifecycle

The configured OpenID Connect provider owns users, credentials, account status,
MFA, passkeys, recovery, and group membership. Lightkite deliberately has no local
user administration screen.

After a successful login, Lightkite keeps a presentation profile keyed by the
standard `issuer + sub` pair. The profile contains display fields, the last
login time, provider groups used for platform-policy evaluation, and dashboard
preferences. Groups are stored as a JSON string array so each claim value is
preserved exactly, including punctuation. It is not an account and cannot
authenticate independently. Upgrading from the legacy comma-separated group
encoding revokes existing sessions; users sign in again.

Grant Kubernetes permissions by binding the provider's exact user or group
identity with native Kubernetes RBAC. Grant maintenance of Lightkite-owned shared
metadata by adding a provider group to `PLATFORM_ADMIN_GROUPS`. Keep these two
permission boundaries separate.

Removing or disabling a person is performed at the identity provider. Existing
Lightkite sessions should also be revoked through logout or operational session
cleanup; provider refresh/token validation prevents a removed identity from
creating a new valid session.
