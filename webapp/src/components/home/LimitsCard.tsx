import { useNav } from '../../context'
import type { CategoryLine } from '../../lib/types'
import { IconCheck } from '../icons'
import { formatAmount } from './format'

/** Categories the report marks as close to / over their limit; over first, then by share; top 3. */
export function nearLimit(lines: CategoryLine[]): CategoryLine[] {
  const share = (l: CategoryLine) => (l.limit ? l.spent / l.limit : 0)
  return lines
    .filter((l) => (l.status === 'warn' || l.status === 'over') && !l.hidden)
    .sort((a, b) => (a.status === b.status ? share(b) - share(a) : a.status === 'over' ? -1 : 1))
    .slice(0, 3)
}

/** «Близко к лимиту» (008 FR-007). */
export function LimitsCard({ lines }: { lines: CategoryLine[] }) {
  const nav = useNav()
  const top = nearLimit(lines)
  return (
    <section className="b-card">
      <div className="b-card-head">
        <h2>Близко к лимиту</h2>
        <button className="b-link" onClick={() => nav.setTab('report')}>
          Все категории
        </button>
      </div>
      {top.length === 0 ? (
        <div className="b-ok">
          <IconCheck size={20} />
          <span>Все лимиты в норме</span>
        </div>
      ) : (
        top.map((l) => {
          const tone = l.status === 'over' ? 'over' : 'warn'
          const fill = l.limit ? Math.min(l.spent / l.limit, 1) : 1
          return (
            <div className="b-limit" key={l.id}>
              <div className="b-limit-row">
                <span className="b-limit-name">{l.label}</span>
                <span className="b-limit-amount num">
                  <b className={tone}>{formatAmount(l.spent)}</b> из {formatAmount(l.limit ?? 0)} с.
                </span>
              </div>
              <div className={'b-bar ' + tone}>
                <div style={{ width: `${fill * 100}%` }} />
              </div>
            </div>
          )
        })
      )}
    </section>
  )
}
