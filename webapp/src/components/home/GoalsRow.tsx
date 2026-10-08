import { useNav } from '../../context'
import type { Goal, QuarterInfo } from '../../lib/types'
import { IconChevron, IconTarget } from '../icons'
import { plural } from './format'

interface Props {
  quarter: QuarterInfo | null | undefined
  /** null while goals are loading or failed to load. */
  goals: Goal[] | null
}

/** «Цели квартала · Q4 2026» row at the end of the main card (008 FR-005). Opens Goals. */
export function GoalsRow({ quarter, goals }: Props) {
  const nav = useNav()
  if (!quarter) return null
  const mine = goals?.filter((g) => g.quarter === quarter.name) ?? null
  const total = mine?.length ?? 0
  const done = mine?.filter((g) => g.status === 'done').length ?? 0
  const left = `ещё ${quarter.daysLeft} ${plural(quarter.daysLeft, ['день', 'дня', 'дней'])}`
  const elapsed = Math.min(Math.max(quarter.elapsed, 0), 1)

  return (
    <button className="b-goals" onClick={() => nav.push({ name: 'goals' })}>
      <span className="b-goals-icon">
        <IconTarget size={18} />
      </span>
      <span className="b-goals-body">
        <span className="b-goals-title">Цели квартала · {quarter.name}</span>
        {mine && total > 0 ? (
          <>
            <span className="b-track">
              <span className="b-fill" style={{ width: `${(done / total) * 100}%` }} />
              <span className="b-tick" style={{ left: `${elapsed * 100}%` }} />
            </span>
            <span className="b-captions">
              <span>
                Выполнено {done} из {total}
              </span>
              <span>{left}</span>
            </span>
          </>
        ) : (
          <span className="b-captions">
            <span>{mine ? 'Поставить цель' : ' '}</span>
            <span>{left}</span>
          </span>
        )}
      </span>
      <IconChevron size={18} className="b-goals-chevron" />
    </button>
  )
}
