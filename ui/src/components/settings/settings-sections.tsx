import type { ComponentType, ReactNode } from 'react'
import type { TFunction } from 'i18next'

import { AuditLog } from './audit-log'
import { ClusterManagement } from './cluster-management'
import { GeneralManagement } from './general-management'
import { TemplateManagement } from './template-management'

export interface SettingsSectionDefinition {
  value: string
  labelKey: string
  defaultLabel: string
  render: () => ReactNode
}

function createSettingsSectionDefinition(
  value: string,
  labelKey: string,
  defaultLabel: string,
  Component: ComponentType
): SettingsSectionDefinition {
  return {
    value,
    labelKey,
    defaultLabel,
    render: () => <Component />,
  }
}

export const settingsSectionRegistry: SettingsSectionDefinition[] = [
  createSettingsSectionDefinition(
    'general',
    'settings.tabs.general',
    'General',
    GeneralManagement
  ),
  createSettingsSectionDefinition(
    'clusters',
    'settings.tabs.clusters',
    'Cluster',
    ClusterManagement
  ),
  createSettingsSectionDefinition(
    'templates',
    'settings.tabs.templates',
    'Templates',
    TemplateManagement
  ),
  createSettingsSectionDefinition(
    'audit',
    'settings.tabs.audit',
    'Audit',
    AuditLog
  ),
]

export function createSettingsTabs(t: TFunction) {
  return settingsSectionRegistry.map((section) => ({
    value: section.value,
    label: t(section.labelKey, section.defaultLabel),
    content: section.render(),
  }))
}
