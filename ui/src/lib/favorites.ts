import { SearchResult } from '@/lib/api'
import { getClusterScopedStorageKey } from '@/lib/current-cluster'

const FAVORITES_STORAGE_KEY = 'kite-favorites'

function favoritesStorageKey(principalKey: string) {
  return getClusterScopedStorageKey(
    `${FAVORITES_STORAGE_KEY}:${encodeURIComponent(principalKey)}`
  )
}

/**
 * Get favorites from localStorage
 */
export const getFavorites = (principalKey = 'local'): SearchResult[] => {
  try {
    const favorites = localStorage.getItem(favoritesStorageKey(principalKey))
    return favorites ? JSON.parse(favorites) : []
  } catch {
    return []
  }
}

/**
 * Save favorites to localStorage
 */
export const saveFavorites = (
  favorites: SearchResult[],
  principalKey = 'local'
) => {
  try {
    localStorage.setItem(
      favoritesStorageKey(principalKey),
      JSON.stringify(favorites)
    )
  } catch (error) {
    console.error('Failed to save favorites:', error)
  }
}

/**
 * Add a resource to favorites
 */
export const addToFavorites = (
  resource: SearchResult,
  principalKey = 'local'
) => {
  const favorites = getFavorites(principalKey)
  const favorite: SearchResult = {
    id: resource.id,
    name: resource.name,
    resourceType: resource.resourceType,
    namespace: resource.namespace,
    createdAt: resource.createdAt,
  }

  // Check if already exists
  if (!favorites.some((fav) => fav.id === favorite.id)) {
    favorites.push(favorite)
    saveFavorites(favorites, principalKey)
  }
}

/**
 * Remove a resource from favorites
 */
export const removeFromFavorites = (
  resourceId: string,
  principalKey = 'local'
) => {
  const favorites = getFavorites(principalKey)
  const filtered = favorites.filter((fav) => fav.id !== resourceId)
  saveFavorites(filtered, principalKey)
}

/**
 * Check if a resource is in favorites
 */
export const isFavorite = (
  resourceId: string,
  principalKey = 'local'
): boolean => {
  const favorites = getFavorites(principalKey)
  return favorites.some((fav) => fav.id === resourceId)
}

/**
 * Toggle favorite status of a resource
 */
export const toggleFavorite = (
  resource: SearchResult,
  principalKey = 'local'
): boolean => {
  if (isFavorite(resource.id, principalKey)) {
    removeFromFavorites(resource.id, principalKey)
    return false
  } else {
    addToFavorites(resource, principalKey)
    return true
  }
}
