import { useRef, useState } from 'react'
import { addTransaction, ApiError, getSummary, newClientId } from '../lib/api'
import { formatSomoni, parseAmountInput } from '../lib/money'
import { haptic } from '../lib/telegram'
import { useAsync } from '../hooks'
import { AmountInput } from '../components/AmountInput'
import { ErrorBanner } from '../components/ErrorBanner'
import { Loader } from '../components/Loader'

type Mode = 'savings_deposit' | 'savings_withdrawal'

/** initial 'deposit' (home «Отложить» / «Пополнить», 008): deposit form preselected, amount focused. */
export function Savings({ initial }: { initial?: 'deposit' } = {}) {
  const summary = useAsync(() => getSummary(), [])
  const [mode, setMode] = useState<Mode>('savings_deposit')
  const [amount, setAmount] = useState('')
  const [note, setNote] = useState('')
  const [touched, setTouched] = useState(false)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<unknown>(null)
  const [done, setDone] = useState<number | null>(null)
  const clientId = useRef(newClientId())

  const s = summary.data
  const parsed = parseAmountInput(amount)
  const inline =
    error instanceof ApiError && (error.code === 'insufficient_savings' || error.field === 'amount')
      ? error.message
      : null

  async function submit() {
    setTouched(true)
    if (!parsed.ok) return
    setSaving(true)
    setError(null)
    try {
      const res = await addTransaction({
        clientId: clientId.current,
        kind: mode,
        amount: parsed.diram,
        note: note.trim() || undefined,
      })
      haptic('success')
      clientId.current = newClientId()
      setDone(res.summary.savingsBalance)
      setAmount('')
      setNote('')
      setTouched(false)
      summary.reload()
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
          ✅ Готово. Накоплено: <b>{formatSomoni(done)}</b>
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
      </div>

      {error != null && !inline && <ErrorBanner error={error} onRetry={submit} />}
      <button className="btn" disabled={!parsed.ok || saving} onClick={submit}>
        {saving ? 'Сохраняю…' : mode === 'savings_deposit' ? 'Пополнить' : 'Снять'}
      </button>
    </div>
  )
}
