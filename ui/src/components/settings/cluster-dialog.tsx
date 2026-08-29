import { useState } from 'react'
import { IconEdit, IconServer } from '@tabler/icons-react'
import { useTranslation } from 'react-i18next'

import { Cluster } from '@/types/api'
import { ClusterCreateRequest } from '@/lib/api'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'

interface ClusterDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  cluster?: Cluster | null
  onSubmit: (clusterData: ClusterCreateRequest) => void
  gatewayEnabled: boolean
  isSubmitting?: boolean
}

function createClusterFormData(cluster?: Cluster | null): ClusterCreateRequest {
  return {
    name: cluster?.name || '',
    description: cluster?.description || '',
    apiServerUrl: cluster?.apiServerUrl || '',
    caBundle: cluster?.caBundle || '',
    tlsServerName: cluster?.tlsServerName || '',
    connectionMode: cluster?.connectionMode || 'direct',
    prometheusURL: cluster?.prometheusURL || '',
    enabled: cluster?.enabled ?? true,
    isDefault: cluster?.isDefault ?? false,
  }
}

export function ClusterDialog({
  open,
  onOpenChange,
  cluster,
  onSubmit,
  gatewayEnabled,
  isSubmitting,
}: ClusterDialogProps) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      {open ? (
        <ClusterDialogContent
          key={cluster?.id ?? 'new'}
          cluster={cluster}
          onOpenChange={onOpenChange}
          onSubmit={onSubmit}
          gatewayEnabled={gatewayEnabled}
          isSubmitting={isSubmitting}
        />
      ) : null}
    </Dialog>
  )
}

function ClusterDialogContent({
  cluster,
  onOpenChange,
  onSubmit,
  gatewayEnabled,
  isSubmitting,
}: Omit<ClusterDialogProps, 'open'>) {
  const { t } = useTranslation()
  const isEditMode = Boolean(cluster)
  const [formData, setFormData] = useState(() => createClusterFormData(cluster))
  const change = <K extends keyof ClusterCreateRequest>(
    key: K,
    value: ClusterCreateRequest[K]
  ) => setFormData((current) => ({ ...current, [key]: value }))
  const direct = formData.connectionMode === 'direct'
  const canSubmit =
    formData.name.trim() !== '' &&
    (formData.connectionMode === 'tunnel' ||
      (direct && formData.apiServerUrl?.trim() !== ''))

  return (
    <DialogContent className="sm:max-w-[640px]">
      <DialogHeader>
        <DialogTitle className="flex items-center gap-2">
          {isEditMode ? (
            <IconEdit className="h-5 w-5" />
          ) : (
            <IconServer className="h-5 w-5" />
          )}
          {isEditMode
            ? t('clusterManagement.dialog.editTitle', 'Edit Cluster')
            : t('clusterManagement.dialog.createTitle', 'Add New Cluster')}
        </DialogTitle>
      </DialogHeader>

      <form
        className="space-y-4"
        onSubmit={(event) => {
          event.preventDefault()
          onSubmit(formData)
        }}
      >
        <div
          className={
            gatewayEnabled
              ? 'grid grid-cols-1 gap-4'
              : 'grid grid-cols-1 gap-4 sm:grid-cols-2'
          }
        >
          <div className="space-y-2">
            <Label htmlFor="cluster-name">
              {t('clusterManagement.dialog.name', 'Cluster Name')} *
            </Label>
            <Input
              id="cluster-name"
              value={formData.name}
              onChange={(event) => change('name', event.target.value)}
              placeholder="production"
              required
            />
          </div>
          {!gatewayEnabled ? (
            <div className="space-y-2">
              <Label htmlFor="cluster-connection-mode">
                {t(
                  'clusterManagement.dialog.connectionMode',
                  'Connection Mode'
                )}
              </Label>
              <Select
                value={formData.connectionMode}
                disabled={isEditMode}
                onValueChange={(value: 'direct' | 'tunnel') =>
                  change('connectionMode', value)
                }
              >
                <SelectTrigger id="cluster-connection-mode">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="direct">
                    {t('clusterManagement.type.direct', 'Direct')}
                  </SelectItem>
                  <SelectItem value="tunnel">
                    {t('clusterManagement.type.tunnel', 'Private Tunnel')}
                  </SelectItem>
                </SelectContent>
              </Select>
            </div>
          ) : null}
        </div>

        <div className="space-y-2">
          <Label htmlFor="cluster-description">
            {t('clusterManagement.dialog.description', 'Description')}
          </Label>
          <Textarea
            id="cluster-description"
            value={formData.description}
            onChange={(event) => change('description', event.target.value)}
            rows={2}
          />
        </div>

        {direct ? (
          <>
            <div className="space-y-2">
              <Label htmlFor="api-server">
                {t(
                  'clusterManagement.dialog.apiServerUrl',
                  'Kubernetes API Server URL'
                )}{' '}
                *
              </Label>
              <Input
                id="api-server"
                type="url"
                value={formData.apiServerUrl}
                onChange={(event) => change('apiServerUrl', event.target.value)}
                placeholder="https://api.cluster.example:6443"
                required
              />
            </div>
            {gatewayEnabled ? (
              <p className="text-xs text-muted-foreground">
                {t(
                  'clusterManagement.dialog.directDescription',
                  'Direct mode requires a publicly reachable API server with a publicly trusted TLS certificate.'
                )}
              </p>
            ) : null}
          </>
        ) : (
          <div className="rounded-lg border border-blue-200 bg-blue-50 p-4 text-sm text-blue-700 dark:border-blue-800 dark:bg-blue-950/20 dark:text-blue-300">
            {t(
              'clusterManagement.dialog.tunnelDescription',
              'After creation, install the generated transport-only agent. It receives no Kubernetes ServiceAccount or RBAC permissions; user ID tokens pass through the tunnel to the API server.'
            )}
          </div>
        )}

        <div className="space-y-2">
          <Label htmlFor="prometheus-url">
            {t('clusterManagement.dialog.prometheusUrl', 'Prometheus URL')}
          </Label>
          <Input
            id="prometheus-url"
            type="url"
            value={formData.prometheusURL}
            onChange={(event) => change('prometheusURL', event.target.value)}
            placeholder={t('clusterManagement.dialog.prometheusUrlPlaceholder')}
          />
          <p className="text-xs text-muted-foreground">
            {t('clusterManagement.dialog.prometheusUrlDescription')}
          </p>
        </div>

        <div className="space-y-4 border-t pt-4">
          <div className="flex items-center justify-between">
            <Label htmlFor="cluster-enabled">
              {t('clusterManagement.dialog.enabled', 'Enable Cluster')}
            </Label>
            <Switch
              id="cluster-enabled"
              checked={formData.enabled}
              onCheckedChange={(value) => change('enabled', value)}
            />
          </div>
          <div className="flex items-center justify-between">
            <Label htmlFor="cluster-default">
              {t('clusterManagement.dialog.isDefault', 'Set as Default')}
            </Label>
            <Switch
              id="cluster-default"
              checked={formData.isDefault}
              onCheckedChange={(value) => change('isDefault', value)}
            />
          </div>
        </div>

        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            onClick={() => onOpenChange(false)}
          >
            {t('common.actions.cancel', 'Cancel')}
          </Button>
          <Button type="submit" disabled={!canSubmit || isSubmitting}>
            {isSubmitting
              ? t('common.messages.saving', 'Saving...')
              : isEditMode
                ? t('common.actions.saveChanges', 'Save Changes')
                : t('clusterManagement.actions.add', 'Add Cluster')}
          </Button>
        </DialogFooter>
      </form>
    </DialogContent>
  )
}
