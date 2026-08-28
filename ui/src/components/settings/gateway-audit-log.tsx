import { useState } from 'react'
import { useAuth } from '@/contexts/auth-context'
import { useTranslation } from 'react-i18next'

import { useGatewayAuditEvents } from '@/lib/api'
import { formatDate } from '@/lib/utils'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'

export function GatewayAuditLog() {
  const { t } = useTranslation()
  const { capabilities } = useAuth()
  const [tokens, setTokens] = useState<string[]>([''])
  const pageToken = tokens[tokens.length - 1]
  const { data, isLoading, error } = useGatewayAuditEvents(pageToken, {
    enabled: capabilities.clusterGatewayEnabled,
  })

  if (!capabilities.clusterGatewayEnabled) return null

  return (
    <Card>
      <CardHeader>
        <CardTitle>
          {t('settings.gatewayAudit.title', 'Gateway access audit')}
        </CardTitle>
        <CardDescription>
          {t(
            'settings.gatewayAudit.description',
            'Review user and Agent requests recorded by Cluster Access Gateway'
          )}
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {error ? (
          <p className="text-destructive text-sm">{error.message}</p>
        ) : (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t('common.fields.time', 'Time')}</TableHead>
                <TableHead>{t('common.fields.actor', 'Actor')}</TableHead>
                <TableHead>{t('common.fields.cluster', 'Cluster')}</TableHead>
                <TableHead>
                  {t('common.fields.operation', 'Operation')}
                </TableHead>
                <TableHead>{t('common.fields.status', 'Status')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {isLoading ? (
                <TableRow>
                  <TableCell colSpan={5} className="text-muted-foreground">
                    {t('common.messages.loading', 'Loading...')}
                  </TableCell>
                </TableRow>
              ) : data?.items.length ? (
                data.items.map((event) => (
                  <TableRow key={event.id}>
                    <TableCell>{formatDate(event.createdAt)}</TableCell>
                    <TableCell>
                      <div className="flex items-center gap-2">
                        <Badge variant="outline">{event.principalType}</Badge>
                        <span
                          className="max-w-64 truncate font-mono text-xs"
                          title={event.tokenId || event.requestId}
                        >
                          {event.agentSubject || event.userSubject}
                        </span>
                      </div>
                    </TableCell>
                    <TableCell>{event.clusterId}</TableCell>
                    <TableCell>
                      <div className="max-w-96">
                        <span className="font-medium">{event.method}</span>{' '}
                        <span className="text-muted-foreground font-mono text-xs">
                          {event.path}
                        </span>
                      </div>
                    </TableCell>
                    <TableCell>
                      <Badge
                        variant={
                          event.status === 0 || event.status < 400
                            ? 'secondary'
                            : 'destructive'
                        }
                      >
                        {event.status === 0
                          ? t('settings.gatewayAudit.inProgress', 'In progress')
                          : event.status}
                      </Badge>
                    </TableCell>
                  </TableRow>
                ))
              ) : (
                <TableRow>
                  <TableCell colSpan={5} className="text-muted-foreground">
                    {t(
                      'settings.gatewayAudit.empty',
                      'No Gateway access events recorded'
                    )}
                  </TableCell>
                </TableRow>
              )}
            </TableBody>
          </Table>
        )}
        <div className="flex justify-end gap-2">
          <Button
            variant="outline"
            disabled={tokens.length === 1}
            onClick={() => setTokens((current) => current.slice(0, -1))}
          >
            {t('common.actions.previous', 'Previous')}
          </Button>
          <Button
            variant="outline"
            disabled={!data?.pagination.nextPageToken}
            onClick={() =>
              setTokens((current) => [
                ...current,
                data?.pagination.nextPageToken ?? '',
              ])
            }
          >
            {t('common.actions.next', 'Next')}
          </Button>
        </div>
      </CardContent>
    </Card>
  )
}
