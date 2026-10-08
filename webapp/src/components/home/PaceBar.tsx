import type { Pace } from '../../lib/types'
import { pct } from './format'

const ELAPSED_LABEL: Record<Pace['period'], string> = {
  month: 'месяца',
  advance: 'до аванса',
  rest: 'до зарплаты',
}

/** Spending pace: share of the budget spent vs share of the period passed (008 FR-004). */
export function PaceBar({ pace }: { pace: Pace | null | undefined }) {
  if (!pace || pace.budget <= 0) return null
  const share = pace.spent / pace.budget
  const tone = share > 1 ? ' over' : share > 0.8 ? ' warn' : ''
  const elapsed = Math.min(Math.max(pace.elapsed, 0), 1)
  return (
    <div className="b-progress">
      <div className="b-track">
        <div className={'b-fill' + tone} style={{ width: `${Math.min(share, 1) * 100}%` }} />
        <div className="b-tick" style={{ left: `${elapsed * 100}%` }} title="сегодня" />
      </div>
      <div className="b-captions">
        <span>Потрачено {pct(share)}% бюджета</span>
        <span>
          прошло {pct(elapsed)}% {ELAPSED_LABEL[pace.period]}
        </span>
      </div>
    </div>
  )
}
