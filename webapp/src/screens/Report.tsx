import { useState } from 'react'
import { useSession } from '../context'
import { getSummary, listTransactions } from '../lib/api'
import { formatSomoni } from '../lib/money'
import { formatDateTime, monthTitle } from '../lib/months'
import type { Kind, Transaction } from '../lib/types'
import { useAsync } from '../hooks'
import { Loader } from '../components/Loader'
import { ErrorBanner } from '../components/ErrorBanner'
import { MonthSwitcher } from '../components/MonthSwitcher'
import { LimitBar } from '../components/LimitBar'

const KIND_ICON: Record<Kind, string> = {
  expense: '💸',
  income: '💵',
  savings_deposit: '🏦',
  savings_withdrawal: '🏦',
  credit_purchase: '💳',
  credit_repayment: '💳',
}

const KIND_LABEL: Record<Kind, string> = {
  expense: 'Расход',
  income: 'Приход',
  savings_deposit: 'В накопления',
  savings_withdrawal: 'Из накоплений',
  credit_purchase: 'По карте',
  credit_repayment: 'Погашение карты',
}

// Sign from the cash-balance point of view; credit purchases don't move cash.
function signed(t: Transaction): string {
  switch (t.kind) {
    case 'income':
    case 'savings_withdrawal':
      return '+' + formatSomoni(t.amount)
    case 'credit_purchase':
      return formatSomoni(t.amount)
    default:
      return '−' + formatSomoni(t.amount)
  }
}

export function Report({ month: initial }: { month: string }) {
  const [month, setMonth] = useState(initial)
  const { categories } = useSession()
  const summary = useAsync(() => getSummary(month), [month])
  const txs = useAsync(() => listTransactions(month), [month])
  const s = summary.data
  const items = txs.data?.items ?? []
  const empty = s !== null && s.income === 0 && s.expense === 0 && txs.data !== null && items.length === 0
  const label = (name?: string | null) => categories.find((c) => c.name === name)?.label ?? name ?? ''

  return (
    <div className="screen">
      <MonthSwitcher month={month} onChange={setMonth} />

      {summary.loading && !s && <Loader />}
      {summary.error != null && <ErrorBanner error={summary.error} onRetry={summary.reload} />}

      {s && s.month === month && (
        <>
          <div className="card">
            <div className="card-title">📊 Отчёт за {monthTitle(month)}</div>
            {s.carryOver !== 0 && (
              <div className="row">
                <span>🔄 Перенос</span>
                <span>{formatSomoni(s.carryOver)}</span>
              </div>
            )}
            <div className="row">
              <span>💵 Приход</span>
              <span>{formatSomoni(s.income)}</span>
            </div>
            <div className="row">
              <span>💸 Расход</span>
              <span>{formatSomoni(s.expense)}</span>
            </div>
            {s.savingsNet !== 0 && (
              <div className="row">
                <span>🏦 Накоплено</span>
                <span>{formatSomoni(s.savingsNet)}</span>
              </div>
            )}
            {(s.creditCharged !== 0 || s.creditRepaid !== 0) && (
              <div className="row">
                <span>💳 По карте</span>
                <span>
                  +{formatSomoni(s.creditCharged)} / −{formatSomoni(s.creditRepaid)}
                </span>
              </div>
            )}
            <div className="row">
              <span>💚 Остаток</span>
              <span className={s.remaining < 0 ? 'negative' : ''}>{formatSomoni(s.remaining)}</span>
            </div>
            {s.savingsBalance > 0 && (
              <div className="row">
                <span>💎 Всего накоплено</span>
                <span>{formatSomoni(s.savingsBalance)}</span>
              </div>
            )}
            {s.creditDebt > 0 && (
              <div className="row">
                <span>💳 Долг</span>
                <span className="negative">−{formatSomoni(s.creditDebt)}</span>
              </div>
            )}
          </div>

          {empty ? (
            <div className="card hint">За этот месяц записей нет</div>
          ) : (
            <div className="card">
              <div className="card-title">Категории</div>
              {s.categories.map((line) => (
                <LimitBar key={line.name} line={line} />
              ))}
            </div>
          )}
        </>
      )}

      {txs.error != null && <ErrorBanner error={txs.error} onRetry={txs.reload} />}
      {items.length > 0 && (
        <details className="card">
          <summary>Записи ({items.length}) ▾</summary>
          {items.map((t) => (
            <div className="tx" key={t.id}>
              <div className="tx-main">
                <div>
                  {KIND_ICON[t.kind]} {t.category ? label(t.category) : KIND_LABEL[t.kind]}
                </div>
                <div className="hint">
                  {formatDateTime(t.createdAt)}
                  {t.note ? ' · ' + t.note : ''}
                </div>
              </div>
              <div className="tx-amount">{signed(t)}</div>
            </div>
          ))}
        </details>
      )}
    </div>
  )
}
