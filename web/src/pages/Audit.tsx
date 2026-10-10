import { useFetch } from '../lib/hooks'
import { useI18n } from '../lib/i18n'
import { formatDate } from '../lib/format'
import type { AuditEntry } from '../lib/types'
import { Badge, Card, Empty, ErrorNote, PageHeader, Table, Td, Th } from '../components/ui'

const tone = (action: string) => (action.includes('delete') ? 'red' : action.includes('create') ? 'green' : action === 'login' ? 'gray' : 'blue')

export default function Audit() {
  const { t, lang } = useI18n()
  const { data, error } = useFetch<AuditEntry[]>('/api/audit?limit=200')
  return (
    <>
      <PageHeader title={t('nav.audit')} />
      <ErrorNote error={error} />
      <Card>
        {data?.length === 0 ? (
          <Empty />
        ) : (
          <Table>
            <thead>
              <tr>
                <Th>{t('audit.time')}</Th>
                <Th>{t('audit.admin')}</Th>
                <Th>{t('audit.action')}</Th>
                <Th>{t('audit.target')}</Th>
                <Th>IP</Th>
              </tr>
            </thead>
            <tbody>
              {data?.map((e) => (
                <tr key={e.id}>
                  <Td className="whitespace-nowrap text-muted-foreground">{formatDate(e.created_at, lang)}</Td>
                  <Td className="font-medium text-foreground">{e.admin}</Td>
                  <Td>
                    <Badge tone={tone(e.action)}>{e.action}</Badge>
                  </Td>
                  <Td className="text-muted-foreground">
                    {e.target}
                    {e.details && <span className="ml-2 text-xs text-faint">{e.details}</span>}
                  </Td>
                  <Td className="font-mono text-xs text-muted-foreground">{e.ip}</Td>
                </tr>
              ))}
            </tbody>
          </Table>
        )}
      </Card>
    </>
  )
}
