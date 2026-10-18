import { Check, Languages } from './icons'
import { ActionsMenu } from './ActionsMenu'
import { LANGS, useI18n } from '../lib/i18n'

// LangMenu is the language button of the pages outside the panel (sign-in,
// subscription): it opens a list of the three languages.
export function LangMenu() {
  const { lang, setLang } = useI18n()
  return (
    <ActionsMenu
      label="language"
      icon={Languages}
      actions={LANGS.map((l) => ({ label: l.label, icon: l.code === lang ? Check : undefined, onClick: () => setLang(l.code) }))}
    />
  )
}
