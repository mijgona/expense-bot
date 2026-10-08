import type { CategoryLine } from '../lib/types'
import { formatSomoni } from '../lib/money'

const ICON: Record<CategoryLine['status'], string> = { ok: '🟢', warn: '🟡', over: '🔴', none: '•' }

export function LimitBar({ line }: { line: CategoryLine }) {
  // Hidden category, unknown key (null) or limit 0 ("без лимита"): amount only, no bar.
  if (line.hidden || line.limit === null || line.limit === 0) {
    return (
      <div className="limit">
        <div className="row">
          <span>
            {ICON.none} {line.label}
          </span>
          <span>
            {formatSomoni(line.spent)}
            {line.hidden ? (
              <span className="tag"> скрыта</span>
            ) : (
              line.limit === 0 && <span className="hint"> · без лимита</span>
            )}
          </span>
        </div>
      </div>
    )
  }
  const pct = line.limit > 0 ? Math.round((line.spent / line.limit) * 100) : 0
  return (
    <div className="limit">
      <div className="row">
        <span>
          {ICON[line.status]} {line.label}
        </span>
        <span>
          {formatSomoni(line.spent)} / {formatSomoni(line.limit)}{' '}
          <span className="hint">({pct}%)</span>
        </span>
      </div>
      <div className={'bar ' + line.status}>
        <span style={{ width: `${Math.min(pct, 100)}%` }} />
      </div>
    </div>
  )
}
