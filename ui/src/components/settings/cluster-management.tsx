import { useCallback, useMemo, useState } from 'react'
import { IconEdit, IconPlus, IconServer, IconTrash } from '@tabler/icons-react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { ColumnDef } from '@tanstack/react-table'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Cluster } from '@/types/api'
import {
  ClusterCreateRequest,
  ClusterUpdateRequest,
  createCluster,
  deleteCluster,
  updateCluster,
  useClusterList,
} from '@/lib/api'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { DeleteConfirmationDialog } from '@/components/delete-confirmation-dialog'

import { Action, ActionTable } from '../action-table'
import { ClusterDialog } from './cluster-dialog'

export function ClusterManagement() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const { data: clusters = [], isLoading, error } = useClusterList()
  const [showClusterDialog, setShowClusterDialog] = useState(false)
  const [editingCluster, setEditingCluster] = useState<Cluster | null>(null)
  const [deletingCluster, setDeletingCluster] = useState<Cluster | null>(null)

  const refreshClusters = useCallback(async () => {
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: ['cluster-list'] }),
      queryClient.invalidateQueries({ queryKey: ['clusters'] }),
    ])
  }, [queryClient])

  const columns = useMemo<ColumnDef<Cluster>[]>(
    () => [
      {
        id: 'name',
        header: t('common.fields.name', 'Name'),
        cell: ({ row: { original: cluster } }) => (
          <div>
            <div className="flex items-center gap-2">
              <span className="font-medium">{cluster.name}</span>
              {cluster.isDefault && <Badge variant="secondary">Default</Badge>}
            </div>
            {cluster.description && (
              <div className="text-sm text-muted-foreground">
                {cluster.description}
              </div>
            )}
          </div>
        ),
      },
      {
        id: 'apiServerUrl',
        header: t('clusterManagement.dialog.apiServerUrl', 'API Server'),
        cell: ({ row: { original: cluster } }) => (
          <span className="font-mono text-xs">{cluster.apiServerUrl}</span>
        ),
      },
      {
        id: 'status',
        header: t('common.fields.status', 'Status'),
        cell: ({ row: { original: cluster } }) =>
          cluster.enabled ? (
            <Badge>{t('status.enabled', 'Enabled')}</Badge>
          ) : (
            <Badge variant="secondary">
              {t('status.disabled', 'Disabled')}
            </Badge>
          ),
      },
      {
        id: 'prometheus',
        header: t('common.fields.prometheus', 'Prometheus'),
        cell: ({ row: { original: cluster } }) => (
          <span className="text-sm text-muted-foreground">
            {cluster.prometheusURL ? 'Yes' : 'No'}
          </span>
        ),
      },
    ],
    [t]
  )

  const actions = useMemo<Action<Cluster>[]>(
    () => [
      {
        label: (
          <>
            <IconEdit className="h-4 w-4" />
            {t('common.actions.edit', 'Edit')}
          </>
        ),
        onClick: (cluster) => {
          setEditingCluster(cluster)
          setShowClusterDialog(true)
        },
      },
      {
        label: (
          <div className="inline-flex items-center gap-2 text-destructive">
            <IconTrash className="h-4 w-4" />
            {t('common.actions.delete', 'Delete')}
          </div>
        ),
        shouldDisable: (cluster) => cluster.isDefault,
        onClick: setDeletingCluster,
      },
    ],
    [t]
  )

  const createMutation = useMutation({
    mutationFn: createCluster,
    onSuccess: async () => {
      await refreshClusters()
      toast.success(
        t('clusterManagement.messages.created', 'Cluster created successfully')
      )
      setShowClusterDialog(false)
    },
    onError: (mutationError: Error) => toast.error(mutationError.message),
  })

  const updateMutation = useMutation({
    mutationFn: ({ id, data }: { id: number; data: ClusterUpdateRequest }) =>
      updateCluster(id, data),
    onSuccess: async () => {
      await refreshClusters()
      toast.success(
        t('clusterManagement.messages.updated', 'Cluster updated successfully')
      )
      setShowClusterDialog(false)
      setEditingCluster(null)
    },
    onError: (mutationError: Error) => toast.error(mutationError.message),
  })

  const deleteMutation = useMutation({
    mutationFn: deleteCluster,
    onSuccess: async () => {
      await refreshClusters()
      toast.success(
        t('clusterManagement.messages.deleted', 'Cluster deleted successfully')
      )
      setDeletingCluster(null)
    },
    onError: (mutationError: Error) => toast.error(mutationError.message),
  })

  const handleSubmitCluster = (clusterData: ClusterCreateRequest) => {
    if (editingCluster) {
      updateMutation.mutate({ id: editingCluster.id, data: clusterData })
      return
    }
    createMutation.mutate(clusterData)
  }

  if (isLoading) {
    return (
      <div className="py-8 text-center text-muted-foreground">
        {t('common.messages.loading', 'Loading...')}
      </div>
    )
  }
  if (error) {
    return (
      <div className="py-8 text-center text-destructive">
        {t('clusterManagement.errors.loadFailed', 'Failed to load clusters')}
      </div>
    )
  }

  return (
    <div className="space-y-6">
      <Card>
        <CardHeader>
          <div className="flex items-center justify-between">
            <CardTitle className="flex items-center gap-2">
              <IconServer className="h-5 w-5" />
              {t('clusterManagement.title', 'Cluster Management')}
            </CardTitle>
            <Button
              className="gap-2"
              onClick={() => {
                setEditingCluster(null)
                setShowClusterDialog(true)
              }}
            >
              <IconPlus className="h-4 w-4" />
              {t('clusterManagement.actions.add', 'Add Cluster')}
            </Button>
          </div>
        </CardHeader>
        <CardContent>
          <ActionTable data={clusters} columns={columns} actions={actions} />
          {clusters.length === 0 && (
            <div className="py-8 text-center text-muted-foreground">
              <IconServer className="mx-auto mb-4 h-12 w-12 opacity-50" />
              <p>
                {t('clusterManagement.empty.title', 'No clusters configured')}
              </p>
              <p className="mt-1 text-sm">
                {t(
                  'clusterManagement.empty.description',
                  'Add your first cluster to get started'
                )}
              </p>
            </div>
          )}
        </CardContent>
      </Card>

      <ClusterDialog
        open={showClusterDialog}
        onOpenChange={(open) => {
          setShowClusterDialog(open)
          if (!open) setEditingCluster(null)
        }}
        cluster={editingCluster}
        onSubmit={handleSubmitCluster}
        isSubmitting={createMutation.isPending || updateMutation.isPending}
      />

      <DeleteConfirmationDialog
        open={Boolean(deletingCluster)}
        onOpenChange={() => setDeletingCluster(null)}
        onConfirm={() => {
          if (deletingCluster) deleteMutation.mutate(deletingCluster.id)
        }}
        resourceName={deletingCluster?.name || ''}
        resourceType="cluster"
        additionalNote={t(
          'clusterManagement.deleteConfirmation',
          "This action will only remove the current cluster's configuration in Kite and will not delete any cluster resources."
        )}
      />
    </div>
  )
}
