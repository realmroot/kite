# Global Search

Kite provides global search for Kubernetes resources by name or exact label.

You can activate the global search via the search bar at the top or by using the shortcut `Ctrl + K` (Windows/Linux) or `Cmd + K` (macOS) on any page.

![Dashboard Overview](/screenshots/global-search.png)

## Features

### Favorites

After clicking the small star on the right side of a resource, you can favorite it. The next time you activate global search, you can quickly find it in the list.

### Search for a specific resource

You can enter the prefix of the resource name followed by a space and the search term you want to input, for example:

```
pod nginx
```

This will only search Pods whose names contain `nginx`.

`pod` can also be abbreviated as `po`.

Supported resource types and abbreviations come from Kite's Kubernetes resource
registry, so aliases stay aligned with the resource pages shipped by this version.

## Limitations

- Fuzzy search is not supported.
- Does not support cross-cluster search.
- Search issues bounded Kubernetes List requests at request time, so very large
  clusters or broad permissions can increase latency.

Each search list runs with the current user's OIDC token. Kubernetes RBAC
therefore controls which resource types and namespaces can contribute results.
Kite intentionally does not cache result sets, so every search re-evaluates the
user's current Kubernetes permissions. Results are capped at 100 entries.
