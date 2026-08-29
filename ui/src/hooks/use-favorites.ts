import { useCallback, useEffect, useState } from 'react'

import { SearchResult } from '@/lib/api'
import {
  addToFavorites as addToFavoritesStorage,
  getFavorites as getFavoritesFromStorage,
  isFavorite as isFavoriteStorage,
  removeFromFavorites as removeFromFavoritesStorage,
  toggleFavorite as toggleFavoriteStorage,
} from '@/lib/favorites'

export function useFavorites(principalKey = 'local') {
  const [favorites, setFavorites] = useState<SearchResult[]>([])
  const [refreshKey, setRefreshKey] = useState(0)

  // Load favorites on mount
  useEffect(() => {
    setFavorites(getFavoritesFromStorage(principalKey))
  }, [principalKey, refreshKey])

  // Refresh favorites list
  const refreshFavorites = useCallback(() => {
    setRefreshKey((prev) => prev + 1)
  }, [])

  // Add to favorites
  const addToFavorites = useCallback(
    (resource: SearchResult) => {
      addToFavoritesStorage(resource, principalKey)
      refreshFavorites()
    },
    [principalKey, refreshFavorites]
  )

  // Remove from favorites
  const removeFromFavorites = useCallback(
    (resourceId: string) => {
      removeFromFavoritesStorage(resourceId, principalKey)
      refreshFavorites()
    },
    [principalKey, refreshFavorites]
  )

  // Check if resource is favorite
  const isFavorite = useCallback(
    (resourceId: string) => {
      return isFavoriteStorage(resourceId, principalKey)
    },
    [principalKey]
  )

  // Toggle favorite status
  const toggleFavorite = useCallback(
    (resource: SearchResult) => {
      const isFavorite = toggleFavoriteStorage(resource, principalKey)
      refreshFavorites()
      return isFavorite
    },
    [principalKey, refreshFavorites]
  )

  return {
    favorites,
    addToFavorites,
    removeFromFavorites,
    isFavorite,
    toggleFavorite,
    refreshFavorites,
  }
}
