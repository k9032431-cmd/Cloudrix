import type { NodeStatus, User, UserStatus } from '../lib/types'
import { useI18n } from '../lib/i18n'
import { relativeTime } from '../lib/format'
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

// Nodes report traffic every ~25s, so two quiet minutes mean nobody is connected.
export const ONLINE_WINDOW = 2 * 60_000
export const isOnline = (iso: string | null) => !!iso && Date.now() - new Date(iso).getTime() < ONLINE_WINDOW

// UserPresenceBadge shows whether an active key is in use right now;
// other states (disabled, limited, expired, on hold) show the status instead.
export function UserPresenceBadge({ user, detail }: { user: Pick<User, 'status' | 'online_at'>; detail?: boolean }) {
  const { t, lang } = useI18n()
  if (user.status !== 'active') return <UserStatusBadge status={user.status} />
  const online = isOnline(user.online_at)
  return (
    <span className="inline-flex items-center gap-2">
      <Badge tone={online ? 'green' : 'gray'} dot>
        {t(online ? 'presence.online' : 'presence.offline')}
      </Badge>
      {detail && !online && (
        <span className="text-xs font-normal text-muted-foreground">
          {user.online_at ? t('presence.lastSeen', { when: relativeTime(user.online_at, lang) }) : t('presence.never')}
        </span>
      )}
    </span>
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
