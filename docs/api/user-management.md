# Current-user preferences API

User lifecycle and account security belong to the configured OIDC provider.
Lightkite does not create, disable, delete, reset, or assign roles to users. A local
profile is created or refreshed after a verified OIDC login and is keyed by
`issuer + sub`.

Lightkite persists only presentation data, last-login time, provider groups for
display/platform-policy evaluation, and dashboard preferences.

## Sidebar preference

```text
POST /api/users/sidebar_preference
```

```json
{
  "sidebar_preference": "<serialized preference>"
}
```

A platform administrator can manage the shared default:

```text
POST   /api/v1/admin/sidebar_preference/global
DELETE /api/v1/admin/sidebar_preference/global
```

The global update uses the same request body. These endpoints affect dashboard
presentation only and never affect Kubernetes access.
