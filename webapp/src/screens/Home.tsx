import { useNav, useSession } from '../context'
import { getSummary } from '../lib/api'
import { formatSomoni } from '../lib/money'
import { useAsync } from '../hooks'
import { Loader } from '../components/Loader'
import { ErrorBanner } from '../components/ErrorBanner'

export function Home() {
  const session = useSession()
  const nav = useNav()
  // Home is re-mounted whenever it becomes the top screen again, so this refetches after every write.
  const { data: s, error, loading, reload } = useAsync(() => getSummary(), [])

  return (
    <div className="screen">
      <div className="screen-title">👋 Привет, {session.user.firstName}!</div>
      <div className="hint">💼 Зарплата: {formatSomoni(session.salary)}</div>

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
        <button className="action" onClick={() => nav.push({ name: 'advisor' })}>
          🤖 ИИ-отчёт
        </button>
      </div>
    </div>
  )
}
