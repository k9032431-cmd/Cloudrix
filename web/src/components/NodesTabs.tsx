import { useNavigate } from 'react-router-dom'
import { useI18n } from '../lib/i18n'
import { Segmented } from './ui'

// NodesTabs switches between nodes and core configurations, as the Nodes
// section does in PasarGuard.
export function NodesTabs({ value }: { value: 'nodes' | 'cores' }) {
  const { t } = useI18n()
  const navigate = useNavigate()
  return (
    <Segmented<'nodes' | 'cores'>
      label={t('nav.nodes')}
      value={value}
      onChange={(v) => navigate(v === 'nodes' ? '/nodes' : '/cores')}
      options={[
        { value: 'nodes', label: t('nav.nodes') },
        { value: 'cores', label: t('nav.cores') },
      ]}
      className="mb-4"
    />
  )
}
