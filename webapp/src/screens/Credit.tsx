import { useRef, useState } from 'react'
import { useNav } from '../context'
import { addTransaction, ApiError, getSummary, newClientId } from '../lib/api'
import { diramToInput, formatSomoni, parseAmountInput } from '../lib/money'
import { haptic } from '../lib/telegram'
import type { Summary } from '../lib/types'
import { useAsync } from '../hooks'
import { AmountInput } from '../components/AmountInput'
import { ErrorBanner } from '../components/ErrorBanner'
import { Loader } from '../components/Loader'

export function Credit() {
  const nav = useNav()
  const summary = useAsync(() => getSummary(), [])
  const [amount, setAmount] = useState('')
  const [note, setNote] = useState('')
  const [touched, setTouched] = useState(false)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<unknown>(null)
  const [done, setDone] = useState<Summary | null>(null)
  const clientId = useRef(newClientId())

  const s = summary.data
  const parsed = parseAmountInput(amount)
  const inline =
    error instanceof ApiError && (error.code === 'exceeds_debt' || error.field === 'amount') ? error.message : null

  async function repay() {
    setTouched(true)
    if (!parsed.ok) return
    setSaving(true)
    setError(null)
    try {
      const res = await addTransaction({
        clientId: clientId.current,
        kind: 'credit_repayment',
        amount: parsed.diram,
        note: note.trim() || undefined,
      })
      haptic('success')
      clientId.current = newClientId()
      setDone(res.summary)
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
      <div className="screen-title">💳 Кредитная карта</div>
      {summary.loading && !s && <Loader />}
      {summary.error != null && <ErrorBanner error={summary.error} onRetry={summary.reload} />}
      {s && (
        <div className="card">
          {s.creditDebt > 0 ? (
            <>
              <div className="hint">💳 Долг</div>
              <div className="big-number negative">{formatSomoni(s.creditDebt)}</div>
            </>
          ) : (
            <div className="card-title">Долга нет 🎉</div>
          )}
          <div className="row">
            <span>В этом месяце</span>
            <span>
              +{formatSomoni(s.creditCharged)} / −{formatSomoni(s.creditRepaid)}
            </span>
          </div>
        </div>
      )}

      <button className="btn" onClick={() => nav.push({ name: 'add', kind: 'credit_purchase' })}>
        ➕ Расход по карте
      </button>

      {done && (
        <div className="banner success">
          <div>✅ Погашено</div>
          <div className="row">
            <span>💳 Долг</span>
            <span>{formatSomoni(done.creditDebt)}</span>
          </div>
          <div className="row">
            <span>💚 Остаток</span>
            <span>{formatSomoni(done.remaining)}</span>
          </div>
        </div>
      )}

      {s && s.creditDebt > 0 && (
        <div className="card">
          <div className="card-title">Погасить</div>
          <AmountInput
            value={amount}
            autoFocus={false}
            onChange={(v) => {
              setAmount(v)
              setTouched(true)
              setDone(null)
            }}
            error={(touched && !parsed.ok ? parsed.error : null) ?? inline}
          />
          <button
            className="btn-link"
            onClick={() => {
              setAmount(diramToInput(s.creditDebt))
              setTouched(true)
            }}
          >
            Вся сумма
          </button>
          <div className="field">
            <label htmlFor="cnote">Описание (необязательно)</label>
            <input id="cnote" className="input" maxLength={200} value={note} onChange={(e) => setNote(e.target.value)} />
          </div>
          {error != null && !inline && <ErrorBanner error={error} onRetry={repay} />}
          <button className="btn" disabled={!parsed.ok || saving} onClick={repay}>
            {saving ? 'Сохраняю…' : 'Погасить'}
          </button>
        </div>
      )}
    </div>
  )
}
