import type { NodeStatus, UserStatus } from '../lib/types'
import { useI18n } from '../lib/i18n'
import { Badge, type Tone } from './ui'

const userTones: Record<UserStatus, Tone> = {
  active: 'green',
  disabled: 'gray',
  limited: 'amber',
  expired: 'red',
  on_hold: 'violet',
}

export function UserStatusBadge({ status }: { status: UserStatus }) {
  const { t } = useI18n()
  return (
    <Badge tone={userTones[status]} dot>
      {t(`status.${status}`)}
    </Badge>
  )
}

const nodeTones: Record<NodeStatus, Tone> = { connected: 'green', connecting: 'amber', error: 'red', disabled: 'gray' }

export function NodeStatusBadge({ status }: { status: NodeStatus }) {
  const { t } = useI18n()
  return (
    <Badge tone={nodeTones[status]} dot>
      {t(`nodes.status.${status}`)}
    </Badge>
  )
}
