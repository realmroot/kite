import { useEffect, useState } from 'react'
import { IconChartBar, IconLink, IconTerminal2 } from '@tabler/icons-react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import {
  updateGeneralSetting,
  useGeneralSetting,
  type GeneralSettingUpdateRequest,
} from '@/lib/api'
import { translateError } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'

const DEFAULT_KUBECTL_IMAGE = 'alpine/kubectl:1.36.3'
const DEFAULT_NODE_TERMINAL_IMAGE = 'busybox:1.37.0'
const DEFAULT_CLUSTER_AGENT_IMAGE = ''

interface GeneralSettingsFormData {
  kubectlEnabled: boolean
  kubectlImage: string
  nodeTerminalImage: string
  clusterAgentImage: string
  enableAnalytics: boolean
  enableVersionCheck: boolean
}

const DEFAULT_FORM_DATA: GeneralSettingsFormData = {
  kubectlEnabled: true,
  kubectlImage: DEFAULT_KUBECTL_IMAGE,
  nodeTerminalImage: DEFAULT_NODE_TERMINAL_IMAGE,
  clusterAgentImage: DEFAULT_CLUSTER_AGENT_IMAGE,
  enableAnalytics: false,
  enableVersionCheck: true,
}

export function GeneralManagement() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const { data, isLoading } = useGeneralSetting()
  const [formData, setFormData] =
    useState<GeneralSettingsFormData>(DEFAULT_FORM_DATA)

  useEffect(() => {
    if (!data) return
    setFormData({
      kubectlEnabled: data.kubectlEnabled,
      kubectlImage: data.kubectlImage || DEFAULT_KUBECTL_IMAGE,
      nodeTerminalImage: data.nodeTerminalImage || DEFAULT_NODE_TERMINAL_IMAGE,
      clusterAgentImage: data.clusterAgentImage || DEFAULT_CLUSTER_AGENT_IMAGE,
      enableAnalytics: data.enableAnalytics,
      enableVersionCheck: data.enableVersionCheck,
    })
  }, [data])

  const mutation = useMutation({
    mutationFn: (payload: GeneralSettingUpdateRequest) =>
      updateGeneralSetting(payload),
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['general-setting'] }),
        queryClient.invalidateQueries({ queryKey: ['bootstrap'] }),
      ])
      toast.success(
        t('generalManagement.messages.updated', 'General settings updated')
      )
    },
    onError: (error) => toast.error(translateError(error, t)),
  })

  const handleSave = () => {
    if (formData.kubectlEnabled && !formData.kubectlImage.trim()) {
      toast.error(
        t(
          'generalManagement.errors.kubectlImageRequired',
          'Kubectl image is required when kubectl is enabled'
        )
      )
      return
    }
    if (!formData.nodeTerminalImage.trim()) {
      toast.error(
        t(
          'generalManagement.errors.nodeTerminalImageRequired',
          'Node terminal image is required'
        )
      )
      return
    }

    mutation.mutate({
      kubectlEnabled: formData.kubectlEnabled,
      kubectlImage: formData.kubectlImage.trim(),
      nodeTerminalImage: formData.nodeTerminalImage.trim(),
      clusterAgentImage:
        formData.clusterAgentImage.trim() || DEFAULT_CLUSTER_AGENT_IMAGE,
      enableAnalytics: formData.enableAnalytics,
      enableVersionCheck: formData.enableVersionCheck,
    })
  }

  if (isLoading && !data) {
    return (
      <div className="py-8 text-center text-muted-foreground">
        {t('common.messages.loading', 'Loading...')}
      </div>
    )
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('generalManagement.title', 'General')}</CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        <section className="rounded-lg border" aria-labelledby="kubectl-title">
          <div className="flex items-center justify-between p-3">
            <div className="space-y-1">
              <Label
                id="kubectl-title"
                htmlFor="general-kubectl-enabled"
                className="flex items-center gap-2 text-sm font-medium"
              >
                <IconTerminal2 className="h-4 w-4" />
                {t('generalManagement.kubectl.title', 'Kubectl')}
              </Label>
              <p className="text-xs text-muted-foreground">
                {t(
                  'generalManagement.kubectl.description',
                  'Enable kubectl terminal and configure runtime image.'
                )}
              </p>
            </div>
            <Switch
              id="general-kubectl-enabled"
              checked={formData.kubectlEnabled}
              onCheckedChange={(kubectlEnabled) =>
                setFormData((current) => ({ ...current, kubectlEnabled }))
              }
            />
          </div>
          {formData.kubectlEnabled && (
            <div className="space-y-2 border-t p-3">
              <Label htmlFor="general-kubectl-image">
                {t('generalManagement.kubectl.form.image', 'Image')}
              </Label>
              <Input
                id="general-kubectl-image"
                value={formData.kubectlImage}
                onChange={(event) =>
                  setFormData((current) => ({
                    ...current,
                    kubectlImage: event.target.value,
                  }))
                }
              />
            </div>
          )}
        </section>

        <section className="space-y-3 rounded-lg border p-3">
          <div className="space-y-1">
            <Label className="flex items-center gap-2 text-sm font-medium">
              <IconTerminal2 className="h-4 w-4" />
              {t('generalManagement.nodeTerminal.title', 'Node Terminal')}
            </Label>
            <p className="text-xs text-muted-foreground">
              {t(
                'generalManagement.nodeTerminal.description',
                'Configure runtime image used for node terminal sessions.'
              )}
            </p>
          </div>
          <Label htmlFor="general-node-terminal-image">
            {t('generalManagement.nodeTerminal.form.image', 'Image')}
          </Label>
          <Input
            id="general-node-terminal-image"
            value={formData.nodeTerminalImage}
            onChange={(event) =>
              setFormData((current) => ({
                ...current,
                nodeTerminalImage: event.target.value,
              }))
            }
          />
        </section>

        <section className="space-y-3 rounded-lg border p-3">
          <div className="space-y-1">
            <Label className="flex items-center gap-2 text-sm font-medium">
              <IconLink className="h-4 w-4" />
              {t('generalManagement.clusterAgent.title', 'Cluster Agent Image')}
            </Label>
            <p className="text-xs text-muted-foreground">
              {t(
                'generalManagement.clusterAgent.description',
                'Container image used for generated Cluster Agent manifests.'
              )}
            </p>
          </div>
          <Label htmlFor="general-cluster-agent-image">
            {t('generalManagement.clusterAgent.form.image', 'Image')}
          </Label>
          <Input
            id="general-cluster-agent-image"
            value={formData.clusterAgentImage}
            onChange={(event) =>
              setFormData((current) => ({
                ...current,
                clusterAgentImage: event.target.value,
              }))
            }
          />
        </section>

        <section className="rounded-lg border" aria-labelledby="runtime-title">
          <div className="p-3">
            <Label
              id="runtime-title"
              className="flex items-center gap-2 text-sm font-medium"
            >
              <IconChartBar className="h-4 w-4" />
              {t('generalManagement.runtime.title', 'Runtime')}
            </Label>
            <p className="mt-1 text-xs text-muted-foreground">
              {t(
                'generalManagement.runtime.description',
                'Configure analytics and version checking behavior.'
              )}
            </p>
          </div>
          <div className="flex items-center justify-between border-t p-3">
            <Label htmlFor="general-enable-analytics">
              {t(
                'generalManagement.runtime.form.enableAnalytics',
                'Enable analytics'
              )}
            </Label>
            <Switch
              id="general-enable-analytics"
              checked={formData.enableAnalytics}
              disabled={!data?.analyticsConfigured}
              onCheckedChange={(enableAnalytics) =>
                setFormData((current) => ({ ...current, enableAnalytics }))
              }
            />
          </div>
          {!data?.analyticsConfigured && (
            <p className="border-t px-3 py-2 text-xs text-muted-foreground">
              {t(
                'generalManagement.runtime.analyticsNotConfigured',
                'Analytics is unavailable until the operator configures a script URL and website ID.'
              )}
            </p>
          )}
          <div className="flex items-center justify-between border-t p-3">
            <Label htmlFor="general-enable-version-check">
              {t(
                'generalManagement.runtime.form.enableVersionCheck',
                'Enable version check'
              )}
            </Label>
            <Switch
              id="general-enable-version-check"
              checked={formData.enableVersionCheck}
              onCheckedChange={(enableVersionCheck) =>
                setFormData((current) => ({
                  ...current,
                  enableVersionCheck,
                }))
              }
            />
          </div>
        </section>

        <div className="flex justify-end">
          <Button onClick={handleSave} disabled={mutation.isPending}>
            {t('common.actions.save', 'Save')}
          </Button>
        </div>
      </CardContent>
    </Card>
  )
}
