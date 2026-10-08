import type { ReactNode } from 'react'
import { useNav, type Tab } from '../context'
import { useTelegramScheme } from '../lib/scheme'
import { IconHistory, IconHome, IconProfile, IconReport } from './icons'
import '../styles-home.css'

const TABS: { tab: Tab; label: string; icon: ReactNode }[] = [
  { tab: 'home', label: 'Главная', icon: <IconHome /> },
  { tab: 'history', label: 'История', icon: <IconHistory /> },
  { tab: 'report', label: 'Отчёт', icon: <IconReport /> },
  { tab: 'profile', label: 'Профиль', icon: <IconProfile /> },
]

/** Bottom tab bar (008 FR-011). Shown only at tab roots; screens opened on top hide it. */
export function TabBar() {
  const nav = useNav()
  const scheme = useTelegramScheme()
  return (
    <nav className="ui-b b-tabbar" data-scheme={scheme} aria-label="Разделы">
      {TABS.map((t) => (
        <button
          key={t.tab}
          className="b-tab"
          aria-current={nav.tab === t.tab ? 'page' : undefined}
          onClick={() => nav.setTab(t.tab)}
        >
          {t.icon}
          <span>{t.label}</span>
        </button>
      ))}
    </nav>
  )
}
