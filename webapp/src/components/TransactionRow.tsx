import { useCategories } from '../lib/categories'
import { formatSomoni } from '../lib/money'
import { formatDate, formatDateTime } from '../lib/months'
import type { Kind, Transaction } from '../lib/types'

export const KIND_ICON: Record<Kind, string> = {
  expense: '💸',
  income: '💵',
  savings_deposit: '🏦',
  savings_withdrawal: '🏦',
  credit_purchase: '💳',
  credit_repayment: '💳',
}

export const KIND_LABEL: Record<Kind, string> = {
  expense: 'Расход',
  income: 'Приход',
  savings_deposit: 'В накопления',
  savings_withdrawal: 'Из накоплений',
  credit_purchase: 'По карте',
  credit_repayment: 'Погашение карты',
}

/** Sign from the cash-balance point of view; credit purchases don't move cash. */
export function signed(t: Pick<Transaction, 'kind' | 'amount'>): string {
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

export function TransactionRow({ tx, onClick }: { tx: Transaction; onClick?: () => void }) {
  const { labelOf } = useCategories()
  const label = tx.category ? labelOf(tx.category) : KIND_LABEL[tx.kind]
  return (
    <div
      className={'tx' + (onClick ? ' clickable' : '')}
      onClick={onClick}
      role={onClick ? 'button' : undefined}
      tabIndex={onClick ? 0 : undefined}
      onKeyDown={onClick ? (e) => e.key === 'Enter' && onClick() : undefined}
    >
      <div className="tx-main">
        <div>
          {KIND_ICON[tx.kind]} {label}
        </div>
        <div className="hint">
          {formatDateTime(tx.occurredAt ?? tx.createdAt)}
          {tx.note ? ' · ' + tx.note : ''}
        </div>
        {tx.editedAt && <div className="tx-edited">изменено {formatDate(tx.editedAt)}</div>}
      </div>
      <div className="tx-amount">{signed(tx)}</div>
    </div>
  )
}
