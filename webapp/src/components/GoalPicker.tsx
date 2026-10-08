import { formatSomoni } from '../lib/money'
import type { Goal } from '../lib/types'

interface Props {
  /** Goals to offer (the caller decides: active ones, plus the record's current goal). */
  goals: Goal[]
  /** Goal ID, '' for «Без цели», null while nothing is chosen. */
  selected: string | null
  onSelect(id: string): void
  error?: string | null
}

/** «На какую цель?» for savings deposits: one chip per goal plus «Без цели». */
export function GoalPicker({ goals, selected, onSelect, error }: Props) {
  return (
    <div className="field">
      <label>На какую цель?</label>
      <div className="chips">
        {goals.map((g) => (
          <button
            key={g.id}
            type="button"
            className={'chip' + (selected === g.id ? ' selected' : '')}
            onClick={() => onSelect(g.id)}
          >
            🎯 {g.name}{' '}
            <span className="hint">
              {formatSomoni(g.saved)} / {formatSomoni(g.target)}
            </span>
          </button>
        ))}
        <button type="button" className={'chip' + (selected === '' ? ' selected' : '')} onClick={() => onSelect('')}>
          Без цели
        </button>
      </div>
      {error && <div className="error-text">{error}</div>}
    </div>
  )
}
