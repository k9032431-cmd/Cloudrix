import { createContext, useCallback, useContext, useState, type ReactNode } from 'react'

export type Lang = 'ru' | 'en'

const ru = {
  'app.name': 'Cloudrix',
  'nav.dashboard': 'Обзор',
  'nav.users': 'Пользователи',
  'nav.inbounds': 'Инбаунды',
  'nav.nodes': 'Ноды',
  'nav.admins': 'Администраторы',
  'nav.audit': 'Журнал',
  'nav.logout': 'Выйти',
  'nav.password': 'Сменить пароль',

  'login.title': 'Вход в панель',
  'login.username': 'Логин',
  'login.password': 'Пароль',
  'login.submit': 'Войти',

  'common.save': 'Сохранить',
  'common.cancel': 'Отмена',
  'common.create': 'Создать',
  'common.edit': 'Изменить',
  'common.delete': 'Удалить',
  'common.close': 'Закрыть',
  'common.copy': 'Копировать',
  'common.copied': 'Скопировано',
  'common.search': 'Поиск…',
  'common.all': 'Все',
  'common.loading': 'Загрузка…',
  'common.empty': 'Пока пусто',
  'common.confirmDelete': 'Удалить «{name}»? Это действие нельзя отменить.',
  'common.unlimited': 'Безлимит',
  'common.never': 'Никогда',
  'common.enabled': 'Включён',
  'common.disabled': 'Выключен',
  'common.generate': 'Сгенерировать',
  'common.actions': 'Действия',
  'common.saved': 'Сохранено',
  'common.prev': 'Назад',
  'common.next': 'Вперёд',

  'status.active': 'Активен',
  'status.disabled': 'Отключён',
  'status.limited': 'Лимит',
  'status.expired': 'Истёк',
  'status.on_hold': 'Ожидает',

  'dash.users': 'Пользователи',
  'dash.active': 'Активные',
  'dash.online': 'Онлайн',
  'dash.traffic': 'Трафик всего',
  'dash.expired': 'Истекшие',
  'dash.limited': 'Достигли лимита',
  'dash.onHold': 'Ожидают',
  'dash.nodes': 'Ноды',
  'dash.trafficChart': 'Трафик за 30 дней',
  'dash.system': 'Система',
  'dash.uptime': 'Аптайм',
  'dash.memory': 'Память',
  'dash.version': 'Версия',

  'users.new': 'Новый пользователь',
  'users.username': 'Имя пользователя',
  'users.status': 'Статус',
  'users.usage': 'Трафик',
  'users.expire': 'Срок',
  'users.online': 'Онлайн',
  'users.dataLimit': 'Лимит трафика, ГБ',
  'users.dataLimitHint': '0 — без ограничений',
  'users.expireAt': 'Действует до',
  'users.expireNone': 'Бессрочно',
  'users.onHoldDays': 'Дней после первого подключения',
  'users.reset': 'Сброс трафика',
  'users.reset.no_reset': 'Никогда',
  'users.reset.day': 'Каждый день',
  'users.reset.week': 'Каждую неделю',
  'users.reset.month': 'Каждый месяц',
  'users.deviceLimit': 'Лимит устройств (HWID)',
  'users.deviceLimitHint': '0 — без ограничений',
  'users.inbounds': 'Инбаунды',
  'users.inboundsHint': 'Ничего не выбрано — доступны все',
  'users.note': 'Заметка',
  'users.owner': 'Владелец',
  'users.daysLeft': '{n} дн.',
  'users.expiredAgo': 'истёк',
  'users.selected': 'Выбрано: {n}',
  'users.bulk.enable': 'Включить',
  'users.bulk.disable': 'Отключить',
  'users.bulk.reset': 'Сбросить трафик',
  'users.bulk.extend': '+30 дней',
  'users.bulk.delete': 'Удалить',
  'users.bulk.confirmDelete': 'Удалить выбранных пользователей ({n})?',
  'users.subscription': 'Подписка',
  'users.links': 'Конфигурации',
  'users.devices': 'Устройства',
  'users.noDevices': 'Устройств ещё нет',
  'users.resetTraffic': 'Сбросить трафик',
  'users.revoke': 'Перевыпустить ключи',
  'users.revokeConfirm': 'Все текущие ссылки и устройства пользователя перестанут работать. Продолжить?',
  'users.lastSub': 'Обновлял подписку',
  'users.lifetime': 'За всё время',
  'users.created': 'Создан',
  'users.total': 'Всего: {n}',

  'inbounds.new': 'Новый инбаунд',
  'inbounds.tag': 'Тег',
  'inbounds.protocol': 'Протокол',
  'inbounds.port': 'Порт',
  'inbounds.listen': 'Адрес прослушивания',
  'inbounds.address': 'Публичный адрес',
  'inbounds.addressHint': 'Пусто — адрес ноды или панели',
  'inbounds.remark': 'Название в клиенте',
  'inbounds.remarkHint': 'Переменные: {USERNAME} {DATA_LEFT} {DAYS_LEFT} {EXPIRE_DATE} {PROTOCOL}',
  'inbounds.node': 'Нода',
  'inbounds.local': 'Локально (панель)',
  'inbounds.transport': 'Транспорт',
  'inbounds.security': 'Безопасность',
  'inbounds.core': 'Ядро',
  'inbounds.preview': 'Конфиг ядра',
  'inbounds.previewHint': 'Сгенерированный серверный конфиг для локального ядра',

  'nodes.new': 'Новая нода',
  'nodes.name': 'Название',
  'nodes.address': 'Адрес',
  'nodes.apiPort': 'Порт API',
  'nodes.coefficient': 'Коэффициент трафика',
  'nodes.status': 'Статус',
  'nodes.lastSeen': 'Был в сети',
  'nodes.disabled': 'Отключить ноду',
  'nodes.agentNote': 'Агент ноды в разработке: ноды пока регистрируются, а их конфиги можно посмотреть на странице инбаундов.',

  'admins.new': 'Новый администратор',
  'admins.role': 'Роль',
  'admins.userLimit': 'Лимит пользователей',
  'admins.passwordHint': 'Оставьте пустым, чтобы не менять',
  'admins.role.sudo': 'Суперадмин',
  'admins.role.admin': 'Админ',
  'admins.role.reseller': 'Реселлер',

  'audit.time': 'Время',
  'audit.admin': 'Админ',
  'audit.action': 'Действие',
  'audit.target': 'Объект',

  'password.current': 'Текущий пароль',
  'password.new': 'Новый пароль',

  'sub.title': 'Ваша подписка',
  'sub.used': 'Использовано',
  'sub.left': 'Осталось',
  'sub.expires': 'Истекает',
  'sub.addToApp': 'Добавить в приложение',
  'sub.copyLink': 'Скопировать ссылку',
  'sub.scan': 'Отсканируйте QR-код в приложении',
  'sub.configs': 'Отдельные конфигурации',
  'sub.download': 'Скачать .conf',
  'sub.notFound': 'Подписка не найдена',
}

type Key = keyof typeof ru

const en: Record<Key, string> = {
  'app.name': 'Cloudrix',
  'nav.dashboard': 'Dashboard',
  'nav.users': 'Users',
  'nav.inbounds': 'Inbounds',
  'nav.nodes': 'Nodes',
  'nav.admins': 'Admins',
  'nav.audit': 'Audit log',
  'nav.logout': 'Sign out',
  'nav.password': 'Change password',

  'login.title': 'Sign in',
  'login.username': 'Username',
  'login.password': 'Password',
  'login.submit': 'Sign in',

  'common.save': 'Save',
  'common.cancel': 'Cancel',
  'common.create': 'Create',
  'common.edit': 'Edit',
  'common.delete': 'Delete',
  'common.close': 'Close',
  'common.copy': 'Copy',
  'common.copied': 'Copied',
  'common.search': 'Search…',
  'common.all': 'All',
  'common.loading': 'Loading…',
  'common.empty': 'Nothing here yet',
  'common.confirmDelete': 'Delete “{name}”? This cannot be undone.',
  'common.unlimited': 'Unlimited',
  'common.never': 'Never',
  'common.enabled': 'Enabled',
  'common.disabled': 'Disabled',
  'common.generate': 'Generate',
  'common.actions': 'Actions',
  'common.saved': 'Saved',
  'common.prev': 'Previous',
  'common.next': 'Next',

  'status.active': 'Active',
  'status.disabled': 'Disabled',
  'status.limited': 'Limited',
  'status.expired': 'Expired',
  'status.on_hold': 'On hold',

  'dash.users': 'Users',
  'dash.active': 'Active',
  'dash.online': 'Online',
  'dash.traffic': 'Total traffic',
  'dash.expired': 'Expired',
  'dash.limited': 'Limited',
  'dash.onHold': 'On hold',
  'dash.nodes': 'Nodes',
  'dash.trafficChart': 'Traffic, last 30 days',
  'dash.system': 'System',
  'dash.uptime': 'Uptime',
  'dash.memory': 'Memory',
  'dash.version': 'Version',

  'users.new': 'New user',
  'users.username': 'Username',
  'users.status': 'Status',
  'users.usage': 'Usage',
  'users.expire': 'Expires',
  'users.online': 'Online',
  'users.dataLimit': 'Data limit, GB',
  'users.dataLimitHint': '0 means unlimited',
  'users.expireAt': 'Expires at',
  'users.expireNone': 'Never',
  'users.onHoldDays': 'Days after first connection',
  'users.reset': 'Traffic reset',
  'users.reset.no_reset': 'Never',
  'users.reset.day': 'Daily',
  'users.reset.week': 'Weekly',
  'users.reset.month': 'Monthly',
  'users.deviceLimit': 'Device limit (HWID)',
  'users.deviceLimitHint': '0 means unlimited',
  'users.inbounds': 'Inbounds',
  'users.inboundsHint': 'None selected means all',
  'users.note': 'Note',
  'users.owner': 'Owner',
  'users.daysLeft': '{n} d',
  'users.expiredAgo': 'expired',
  'users.selected': 'Selected: {n}',
  'users.bulk.enable': 'Enable',
  'users.bulk.disable': 'Disable',
  'users.bulk.reset': 'Reset traffic',
  'users.bulk.extend': '+30 days',
  'users.bulk.delete': 'Delete',
  'users.bulk.confirmDelete': 'Delete selected users ({n})?',
  'users.subscription': 'Subscription',
  'users.links': 'Configs',
  'users.devices': 'Devices',
  'users.noDevices': 'No devices yet',
  'users.resetTraffic': 'Reset traffic',
  'users.revoke': 'Rotate keys',
  'users.revokeConfirm': 'All current links and devices of this user will stop working. Continue?',
  'users.lastSub': 'Last subscription update',
  'users.lifetime': 'Lifetime',
  'users.created': 'Created',
  'users.total': 'Total: {n}',

  'inbounds.new': 'New inbound',
  'inbounds.tag': 'Tag',
  'inbounds.protocol': 'Protocol',
  'inbounds.port': 'Port',
  'inbounds.listen': 'Listen address',
  'inbounds.address': 'Public address',
  'inbounds.addressHint': 'Empty means node or panel address',
  'inbounds.remark': 'Client display name',
  'inbounds.remarkHint': 'Variables: {USERNAME} {DATA_LEFT} {DAYS_LEFT} {EXPIRE_DATE} {PROTOCOL}',
  'inbounds.node': 'Node',
  'inbounds.local': 'Local (panel)',
  'inbounds.transport': 'Transport',
  'inbounds.security': 'Security',
  'inbounds.core': 'Core',
  'inbounds.preview': 'Core config',
  'inbounds.previewHint': 'Generated server config for the local core',

  'nodes.new': 'New node',
  'nodes.name': 'Name',
  'nodes.address': 'Address',
  'nodes.apiPort': 'API port',
  'nodes.coefficient': 'Usage coefficient',
  'nodes.status': 'Status',
  'nodes.lastSeen': 'Last seen',
  'nodes.disabled': 'Disable node',
  'nodes.agentNote': 'The node agent is in development: nodes can be registered now, and their configs previewed on the Inbounds page.',

  'admins.new': 'New admin',
  'admins.role': 'Role',
  'admins.userLimit': 'User limit',
  'admins.passwordHint': 'Leave empty to keep the current one',
  'admins.role.sudo': 'Sudo',
  'admins.role.admin': 'Admin',
  'admins.role.reseller': 'Reseller',

  'audit.time': 'Time',
  'audit.admin': 'Admin',
  'audit.action': 'Action',
  'audit.target': 'Target',

  'password.current': 'Current password',
  'password.new': 'New password',

  'sub.title': 'Your subscription',
  'sub.used': 'Used',
  'sub.left': 'Left',
  'sub.expires': 'Expires',
  'sub.addToApp': 'Add to app',
  'sub.copyLink': 'Copy link',
  'sub.scan': 'Scan the QR code in your app',
  'sub.configs': 'Individual configs',
  'sub.download': 'Download .conf',
  'sub.notFound': 'Subscription not found',
}

const dicts: Record<Lang, Record<Key, string>> = { ru, en }

export type T = (key: Key, vars?: Record<string, string | number>) => string

interface I18n {
  lang: Lang
  setLang: (l: Lang) => void
  t: T
}

const Ctx = createContext<I18n | null>(null)

function initialLang(): Lang {
  try {
    const saved = localStorage.getItem('lang')
    if (saved === 'ru' || saved === 'en') return saved
  } catch {
    /* ignore */
  }
  return navigator.language.startsWith('ru') ? 'ru' : 'en'
}

export function I18nProvider({ children }: { children: ReactNode }) {
  const [lang, setLangState] = useState<Lang>(initialLang)
  const setLang = useCallback((l: Lang) => {
    setLangState(l)
    document.documentElement.lang = l
    try {
      localStorage.setItem('lang', l)
    } catch {
      /* ignore */
    }
  }, [])
  const t = useCallback<T>(
    (key, vars) => {
      let s = dicts[lang][key] ?? key
      if (vars) for (const [k, v] of Object.entries(vars)) s = s.replace(`{${k}}`, String(v))
      return s
    },
    [lang],
  )
  return <Ctx.Provider value={{ lang, setLang, t }}>{children}</Ctx.Provider>
}

export function useI18n() {
  const ctx = useContext(Ctx)
  if (!ctx) throw new Error('useI18n outside provider')
  return ctx
}
