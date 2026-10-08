import type { ReactNode } from 'react'
import type { Summary } from '../../lib/types'
import { ApiError } from '../../lib/api'
import { PaceBar } from './PaceBar'
import { days, formatAmount, monthWord } from './format'

interface Props {
  summary: Summary | null
  loading: boolean
  error: unknown
  onRetry(): void
  /** Goals row rendered at the end of the card (008 FR-005). */
  goals?: ReactNode
}

function Rings() {
  return (
    <svg className="b-rings" aria-hidden="true" width="240" height="240" viewBox="0 0 240 240">
      <circle cx="120" cy="120" r="44" />
      <circle cx="120" cy="120" r="68" />
      <circle cx="120" cy="120" r="92" />
      <circle cx="120" cy="120" r="116" />
    </svg>
  )
}

/** Balance, «Можно в день», days left, pace bar and quarter goals (008 US1). */
export function MainCard({ summary: s, loading, error, onRetry, goals }: Props) {
  if (!s) {
    return (
      <section className="b-main-card" aria-busy={loading}>
        <Rings />
        {error != null && !loading ? (
          <div className="b-card-error" role="alert">
            <span>{error instanceof ApiError ? error.message : 'Не удалось загрузить остаток'}</span>
            <button className="b-btn-light" onClick={onRetry}>
              Повторить
            </button>
          </div>
        ) : (
          <>
            <div className="b-skel" style={{ width: 150, height: 15 }} />
            <div className="b-skel" style={{ width: 210, height: 36 }} />
            <div className="b-stats">
              <div className="b-skel" style={{ height: 58, borderRadius: 16 }} />
              <div className="b-skel" style={{ height: 58, borderRadius: 16 }} />
            </div>
            <div className="b-skel" style={{ height: 6 }} />
          </>
        )}
      </section>
    )
  }

  const overspent = s.remaining < 0
  const amount = formatAmount(s.remaining)
  const size = amount.length > 12 ? ' xlong' : amount.length > 9 ? ' long' : ''
  const perDay = Math.max(0, s.dailyBudget ?? 0)
  const np = s.nextPayday
  const daysLabel = np ? (np.kind === 'advance' ? 'До аванса' : 'До зарплаты') : 'До конца месяца'
  const daysLeft = np ? np.daysLeft : s.daysLeft

  return (
    <section className="b-main-card">
      <Rings />
      <span className="b-main-label">
        {overspent
          ? 'Перерасход'
          : np
            ? np.kind === 'advance'
              ? 'Остаток до аванса'
              : 'Остаток до зарплаты'
            : `Остаток на ${monthWord(s.month)}`}
      </span>
      <div className="b-balance num display">
        <span className={'b-balance-value' + size}>{amount}</span>
        <span className="b-balance-cur">с.</span>
      </div>
      <div className="b-stats">
        <div className="b-stat">
          <span className="b-stat-label">Можно в день</span>
          <span className="b-stat-value num display">{formatAmount(perDay)} с.</span>
        </div>
        <div className="b-stat">
          <span className="b-stat-label">{daysLabel}</span>
          <span className="b-stat-value num display">{daysLeft != null ? days(daysLeft) : '—'}</span>
        </div>
      </div>
      <PaceBar pace={s.pace} />
      {goals && (
        <>
          <div className="b-divider" />
          {goals}
        </>
      )}
    </section>
  )
}
