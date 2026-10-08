import { useRef, useState } from 'react'
import { addTransaction, ApiError, getSummary, listGoals, newClientId } from '../lib/api'
import { formatSomoni, parseAmountInput } from '../lib/money'
import { haptic } from '../lib/telegram'
import { useAsync } from '../hooks'
import { AmountInput } from '../components/AmountInput'
import { ErrorBanner } from '../components/ErrorBanner'
import { GoalPicker } from '../components/GoalPicker'
import { Loader } from '../components/Loader'

type Mode = 'savings_deposit' | 'savings_withdrawal'

/** initial 'deposit' (home «Отложить» / «Пополнить», 008): deposit form preselected, amount focused. */
export function Savings({ initial }: { initial?: 'deposit' } = {}) {
  const summary = useAsync(() => getSummary(), [])
  const goals = useAsync(() => listGoals(), [])
  const [mode, setMode] = useState<Mode>('savings_deposit')
  const [amount, setAmount] = useState('')
  const [note, setNote] = useState('')
  const [touched, setTouched] = useState(false)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<unknown>(null)
  const [done, setDone] = useState<{ balance: number; goal: string | null } | null>(null)
  // '' = «Без цели», null = not chosen yet (asked on every deposit while active goals exist).
  const [goalId, setGoalId] = useState<string | null>(null)
  const clientId = useRef(newClientId())

  const s = summary.data
  const parsed = parseAmountInput(amount)
  const activeGoals = goals.data?.items.filter((g) => g.status !== 'done') ?? []
  const asksGoal = mode === 'savings_deposit' && activeGoals.length > 0
  const goalError =
    error instanceof ApiError && error.field === 'goalId'
      ? error.message
      : touched && asksGoal && goalId === null
        ? 'Выберите цель или «Без цели»'
        : null
  const inline =
    error instanceof ApiError && (error.code === 'insufficient_savings' || error.field === 'amount')
      ? error.message
      : null

  async function submit() {
    setTouched(true)
    if (!parsed.ok || (asksGoal && goalId === null)) return
    const goal = asksGoal && goalId ? goalId : undefined
    setSaving(true)
    setError(null)
    try {
      const res = await addTransaction({
        clientId: clientId.current,
        kind: mode,
        amount: parsed.diram,
        note: note.trim() || undefined,
        goalId: goal,
      })
      haptic('success')
      clientId.current = newClientId()
      setDone({ balance: res.summary.savingsBalance, goal: activeGoals.find((g) => g.id === goal)?.name ?? null })
      setAmount('')
      setNote('')
      setGoalId(null)
      setTouched(false)
      summary.reload()
      goals.reload()
    } catch (e) {
      haptic('error')
      setError(e)
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="screen">
      <div className="screen-title">🏦 Накопления</div>
      {summary.loading && !s && <Loader />}
      {summary.error != null && <ErrorBanner error={summary.error} onRetry={summary.reload} />}
      {s && (
        <div className="card">
          <div className="hint">💎 Всего накоплено</div>
          <div className="big-number">{formatSomoni(s.savingsBalance)}</div>
          <div className="row">
            <span>🏦 В этом месяце</span>
            <span>{formatSomoni(s.savingsNet)}</span>
          </div>
        </div>
      )}

      {done !== null && (
        <div className="banner success">
          ✅ Готово.{done.goal ? ` На цель «${done.goal}».` : ''} Накоплено: <b>{formatSomoni(done.balance)}</b>
        </div>
      )}

      <div className="segmented">
        <button
          className={mode === 'savings_deposit' ? 'active' : ''}
          onClick={() => {
            setMode('savings_deposit')
            setError(null)
          }}
        >
          Пополнить
        </button>
        <button
          className={mode === 'savings_withdrawal' ? 'active' : ''}
          onClick={() => {
            setMode('savings_withdrawal')
            setError(null)
          }}
        >
          Снять
        </button>
      </div>

      <div className="card">
        <AmountInput
          value={amount}
          autoFocus={initial === 'deposit'}
          onChange={(v) => {
            setAmount(v)
            setTouched(true)
            setDone(null)
          }}
          error={(touched && !parsed.ok ? parsed.error : null) ?? inline}
        />
        <div className="field">
          <label htmlFor="snote">Описание (необязательно)</label>
          <input id="snote" className="input" maxLength={200} value={note} onChange={(e) => setNote(e.target.value)} />
        </div>
        {asksGoal && (
          <GoalPicker
            goals={activeGoals}
            selected={goalId}
            onSelect={(id) => {
              setGoalId(id)
              setDone(null)
              if (error instanceof ApiError && error.field === 'goalId') setError(null)
            }}
            error={goalError}
          />
        )}
      </div>

      {error != null && !inline && !(error instanceof ApiError && error.field === 'goalId') && <ErrorBanner error={error} onRetry={submit} />}
      <button className="btn" disabled={!parsed.ok || saving} onClick={submit}>
        {saving ? 'Сохраняю…' : mode === 'savings_deposit' ? 'Пополнить' : 'Снять'}
      </button>
    </div>
  )
}
