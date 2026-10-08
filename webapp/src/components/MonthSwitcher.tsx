import { useSession } from '../context'
import { monthTitle, nextMonth, prevMonth } from '../lib/months'

export function MonthSwitcher({ month, onChange }: { month: string; onChange(month: string): void }) {
  const { currentMonth, firstMonth } = useSession()
  // "YYYY-MM" strings compare correctly lexicographically.
  const canPrev = firstMonth !== null && month > firstMonth
  const canNext = month < currentMonth
  return (
    <div className="month-switcher">
      <button aria-label="Предыдущий месяц" disabled={!canPrev} onClick={() => onChange(prevMonth(month))}>
        ‹
      </button>
      <div>{monthTitle(month)}</div>
      <button aria-label="Следующий месяц" disabled={!canNext} onClick={() => onChange(nextMonth(month))}>
        ›
      </button>
    </div>
  )
}
