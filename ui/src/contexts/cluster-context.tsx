/* eslint-disable react-refresh/only-export-components */
import React, { createContext, useEffect, useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'

import { Cluster } from '@/types/api'
import { useCurrentClusterList } from '@/lib/api/cluster'
import {
  clearCurrentCluster,
  getCurrentCluster,
  setCurrentCluster as persistCurrentCluster,
} from '@/lib/current-cluster'

interface ClusterContextType {
  clusters: Cluster[]
  currentCluster: string | null
  setCurrentCluster: (clusterName: string) => void
  isLoading: boolean
  isSwitching?: boolean
  error: Error | null
}

export const ClusterContext = createContext<ClusterContextType | undefined>(
  undefined
)

const shouldRefreshForClusterSwitch = (query: {
  queryKey: readonly unknown[]
}) => {
  const key = query.queryKey[0]
  return typeof key === 'string' && !['user', 'auth', 'clusters'].includes(key)
}

export const ClusterProvider: React.FC<{ children: React.ReactNode }> = ({
  children,
}) => {
  const [currentCluster, setCurrentClusterState] = useState<string | null>(
    getCurrentCluster()
  )
  const [isSwitching, setIsSwitching] = useState(false)
  const queryClient = useQueryClient()
  const {
    data: clusters = [],
    isLoading,
    error: queryError,
  } = useCurrentClusterList()
  const error = queryError instanceof Error ? queryError : null

  useEffect(() => {
    if (currentCluster) {
      persistCurrentCluster(currentCluster)
      return
    }
    clearCurrentCluster()
  }, [currentCluster])

  useEffect(() => {
    if (clusters.length === 0) {
      return
    }

    if (
      !currentCluster ||
      !clusters.some((cluster) => cluster.name === currentCluster)
    ) {
      const defaultCluster = clusters.find((cluster) => cluster.isDefault)
      const nextCluster = defaultCluster
        ? defaultCluster.name
        : clusters[0].name
      setCurrentClusterState(nextCluster)
      persistCurrentCluster(nextCluster)

      void queryClient
        .resetQueries({ predicate: shouldRefreshForClusterSwitch })
        .catch(() => {
          toast.error('Failed to load the selected cluster', {
            id: 'cluster-switch',
          })
        })
    }
  }, [clusters, currentCluster, queryClient])

  const setCurrentCluster = async (clusterName: string) => {
    if (clusterName === currentCluster || isSwitching) {
      return
    }

    setIsSwitching(true)
    setCurrentClusterState(clusterName)
    persistCurrentCluster(clusterName)

    try {
      await queryClient.resetQueries({
        predicate: shouldRefreshForClusterSwitch,
      })
      toast.success(`Switched to cluster: ${clusterName}`, {
        id: 'cluster-switch',
      })
    } catch (switchError) {
      console.error('Failed to switch cluster:', switchError)
      toast.error('Failed to switch cluster', {
        id: 'cluster-switch',
      })
    } finally {
      setIsSwitching(false)
    }
  }

  const value: ClusterContextType = {
    clusters,
    currentCluster,
    setCurrentCluster,
    isLoading,
    isSwitching,
    error,
  }

  return (
    <ClusterContext.Provider value={value}>{children}</ClusterContext.Provider>
  )
}
