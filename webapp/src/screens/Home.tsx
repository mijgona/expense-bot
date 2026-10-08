import { useState } from 'react'
import { useNav, useSession } from '../context'
import { getSummary, listPayouts } from '../lib/api'
import { formatSomoni } from '../lib/money'
import { takePayoutLink } from '../lib/payouts'
import type { Payout } from '../lib/types'
import { useAsync } from '../hooks'
import { Loader } from '../components/Loader'
import { ErrorBanner } from '../components/ErrorBanner'
import { PayoutOffer, PayoutRecorded } from '../components/PayoutOffer'

const samePayout = (a: Payout, month: string, kind: string) => a.month === month && a.kind === kind

export function Home() {
  const session = useSession()
  const nav = useNav()
  // Home is re-mounted whenever it becomes the top screen again, so this refetches after every write.
  const { data: s, error, loading, reload } = useAsync(() => getSummary(), [])
  const payouts = useAsync(() => listPayouts(), [])
  // Reminder deep link (?payout=YYYY-MM_kind), handed over once per app launch.
  const [link, setLink] = useState(() => takePayoutLink())
  // The link may point at a month that /api/payouts (current month) doesn't cover.
  const linked = useAsync(
    () =>
      link && link.month !== session.currentMonth ? listPayouts(link.month) : Promise.resolve(null),
    [link?.month],
  )

  const items = payouts.data?.items ?? []
  const linkItem = link
    ? (items.find((p) => samePayout(p, link.month, link.kind)) ??
      linked.data?.items.find((p) => samePayout(p, link.month, link.kind)) ??
      null)
    : null
  const due = items.filter((p) => p.status === 'due' && !(linkItem && samePayout(p, linkItem.month, linkItem.kind)))

  function afterPayout() {
    setLink(null)
    reload()
    payouts.reload()
  }

  const np = s?.nextPayday ?? null

  return (
    <div className="screen">
      <div className="screen-title">👋 Привет, {session.user.displayName || session.user.firstName}!</div>
      <div className="hint">💼 Зарплата: {formatSomoni(session.salary)}</div>

      {linkItem &&
        (linkItem.status === 'recorded' ? (
          <PayoutRecorded
            payout={linkItem}
            onOpenHistory={() => nav.push({ name: 'history', filter: { month: linkItem.month, group: 'income' } })}
          />
        ) : (
          <PayoutOffer key={`link-${linkItem.month}-${linkItem.kind}`} payout={linkItem} autoOpen onChanged={afterPayout} />
        ))}
      {due.map((p) => (
        <PayoutOffer key={`${p.month}-${p.kind}`} payout={p} onChanged={afterPayout} />
      ))}

      {loading && !s && <Loader text="Считаю баланс…" />}
      {error != null && <ErrorBanner error={error} onRetry={reload} />}
      {s && (
        <div className="card">
          <div className="hint">💚 Остаток</div>
          <div className={'big-number' + (s.remaining < 0 ? ' negative' : '')}>{formatSomoni(s.remaining)}</div>
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
          {np ? (
            <div className="row">
              <span>
                📅 {np.kind === 'advance' ? 'До аванса' : 'До зарплаты'} {np.daysLeft} дн.
              </span>
              <span>{s.dailyBudget != null ? `на день ${formatSomoni(s.dailyBudget)}` : ''}</span>
            </div>
          ) : (
            <>
              {s.daysLeft != null && (
                <div className="row">
                  <span>📅 Осталось дней</span>
                  <span>{s.daysLeft}</span>
                </div>
              )}
              {s.dailyBudget != null && (
                <div className="row">
                  <span>📊 На день</span>
                  <span>{formatSomoni(s.dailyBudget)}</span>
                </div>
              )}
            </>
          )}
          {s.savingsBalance > 0 && (
            <div className="row">
              <span>💎 Всего накоплено</span>
              <span>{formatSomoni(s.savingsBalance)}</span>
            </div>
          )}
          {s.creditDebt > 0 && (
            <div className="row">
              <span>💳 Долг по карте</span>
              <span className="negative">−{formatSomoni(s.creditDebt)}</span>
            </div>
          )}
        </div>
      )}

      <div className="actions">
        <button className="action action-primary" onClick={() => nav.push({ name: 'add', kind: 'expense' })}>
          ➕ Расход
        </button>
        <button className="action" onClick={() => nav.push({ name: 'add', kind: 'income' })}>
          💵 Приход
        </button>
        <button className="action" onClick={() => nav.push({ name: 'report', month: session.currentMonth })}>
          📊 Отчёт
        </button>
        <button className="action" onClick={() => nav.push({ name: 'savings' })}>
          🏦 Накопления
        </button>
        <button className="action" onClick={() => nav.push({ name: 'credit' })}>
          💳 Карта
        </button>
        <button className="action" onClick={() => nav.push({ name: 'goals' })}>
          🎯 Цели
        </button>
        <button className="action" onClick={() => nav.push({ name: 'history' })}>
          📜 История
        </button>
        <button className="action" onClick={() => nav.push({ name: 'advisor' })}>
          🤖 ИИ-отчёт
        </button>
        <button className="action" onClick={() => nav.push({ name: 'profile' })}>
          👤 Профиль
        </button>
      </div>
    </div>
  )
}
