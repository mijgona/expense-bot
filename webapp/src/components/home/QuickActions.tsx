import type { ReactNode } from 'react'
import { useNav, type Screen } from '../../context'
import { IconCard, IconExpense, IconIncome, IconSave, IconSparkle } from '../icons'

const ACTIONS: { label: string; icon: ReactNode; screen: Screen; primary?: boolean }[] = [
  { label: 'Расход', icon: <IconExpense />, screen: { name: 'add', kind: 'expense' }, primary: true },
  { label: 'Приход', icon: <IconIncome />, screen: { name: 'add', kind: 'income' } },
  { label: 'По карте', icon: <IconCard />, screen: { name: 'add', kind: 'credit_purchase' } },
  { label: 'Отложить', icon: <IconSave />, screen: { name: 'savings', mode: 'deposit' } },
  { label: 'ИИ-разбор', icon: <IconSparkle />, screen: { name: 'advisor' } },
]

/** Five one-tap entries under the main card (008 FR-006). */
export function QuickActions() {
  const nav = useNav()
  return (
    <nav className="b-quick" aria-label="Быстрые действия">
      {ACTIONS.map((a) => (
        <button key={a.label} className={'b-quick-btn' + (a.primary ? ' primary' : '')} onClick={() => nav.push(a.screen)}>
          <span className="b-quick-tile">{a.icon}</span>
          <span className="b-quick-label">{a.label}</span>
        </button>
      ))}
    </nav>
  )
}
