import { useRef, useState } from 'react'
import { useSession } from '../context'
import { addGoal, ApiError, listGoals, newClientId } from '../lib/api'
import { formatSomoni, parseAmountInput } from '../lib/money'
import { allowedQuarters } from '../lib/months'
import { haptic } from '../lib/telegram'
import { useAsync } from '../hooks'
import { AmountInput } from '../components/AmountInput'
import { ErrorBanner } from '../components/ErrorBanner'
import { Loader } from '../components/Loader'

export function Goals() {
  const { currentMonth } = useSession()
  const goals = useAsync(() => listGoals(), [])
  const quarters = allowedQuarters(currentMonth)

  const [formOpen, setFormOpen] = useState(false)
  const [name, setName] = useState('')
  const [target, setTarget] = useState('')
  const [quarter, setQuarter] = useState(quarters[0])
  const [note, setNote] = useState('')
  const [touched, setTouched] = useState(false)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<unknown>(null)
  const clientId = useRef(newClientId())

  const parsed = parseAmountInput(target)
  const field = error instanceof ApiError && error.code === 'validation' ? error.field : undefined
  const nameError = touched && !name.trim() ? 'Введите название' : field === 'name' ? (error as ApiError).message : null
  const targetError = touched && !parsed.ok ? parsed.error : field === 'target' ? (error as ApiError).message : null
  const valid = name.trim().length > 0 && name.trim().length <= 60 && parsed.ok

  async function save() {
    setTouched(true)
    if (!valid || !parsed.ok) return
    setSaving(true)
    setError(null)
    try {
      await addGoal({
        clientId: clientId.current,
        name: name.trim(),
        target: parsed.diram,
        quarter,
        note: note.trim() || undefined,
      })
      haptic('success')
      clientId.current = newClientId()
      setName('')
      setTarget('')
      setNote('')
      setQuarter(quarters[0])
      setTouched(false)
      setFormOpen(false)
      goals.reload()
    } catch (e) {
      haptic('error')
      setError(e)
    } finally {
      setSaving(false)
    }
  }

  const g = goals.data
  return (
    <div className="screen">
      <div className="screen-title">🎯 Цели на квартал</div>
      {goals.loading && !g && <Loader />}
      {goals.error != null && <ErrorBanner error={goals.error} onRetry={goals.reload} />}

      {g && g.items.length === 0 && <div className="card hint">Целей ещё нет. Добавь первую!</div>}
      {g?.items.map((goal) => {
        const pct = Math.round(goal.progress * 100)
        return (
          <div className="card" key={goal.id}>
            <div className="card-title">
              {goal.status === 'done' ? '✅' : '🔵'} {goal.name}
            </div>
            <div className="row">
              <span>
                💰 {formatSomoni(goal.target)} · 📅 {goal.quarter}
              </span>
              <span>{pct}%</span>
            </div>
            <div className="bar progress">
              <span style={{ width: `${pct}%` }} />
            </div>
            <div className="hint">💎 из накоплений {formatSomoni(g.savingsBalance)}</div>
            {goal.note && <div className="hint">📝 {goal.note}</div>}
          </div>
        )
      })}

      {!formOpen ? (
        <button className="btn" onClick={() => setFormOpen(true)}>
          ➕ Новая цель
        </button>
      ) : (
        <div className="card">
          <div className="card-title">Новая цель</div>
          <div className="field">
            <label htmlFor="gname">Название</label>
            <input
              id="gname"
              className={'input' + (nameError ? ' invalid' : '')}
              maxLength={60}
              placeholder="например, Отпуск"
              autoFocus
              value={name}
              onChange={(e) => setName(e.target.value)}
            />
            {nameError && <div className="error-text">{nameError}</div>}
          </div>
          <AmountInput
            label="Целевая сумма, с."
            autoFocus={false}
            value={target}
            onChange={(v) => {
              setTarget(v)
              setTouched(true)
            }}
            error={targetError}
          />
          <div className="field">
            <label>Квартал</label>
            <div className="chips">
              {quarters.map((q) => (
                <button
                  key={q}
                  type="button"
                  className={'chip' + (q === quarter ? ' selected' : '')}
                  onClick={() => setQuarter(q)}
                >
                  {q}
                </button>
              ))}
            </div>
            {field === 'quarter' && <div className="error-text">{(error as ApiError).message}</div>}
          </div>
          <div className="field">
            <label htmlFor="gnote">Описание (необязательно)</label>
            <input id="gnote" className="input" maxLength={200} value={note} onChange={(e) => setNote(e.target.value)} />
          </div>
          {error != null && !field && <ErrorBanner error={error} onRetry={save} />}
          <button className="btn" disabled={!valid || saving} onClick={save}>
            {saving ? 'Сохраняю…' : 'Сохранить цель'}
          </button>
          <button className="btn-link" onClick={() => setFormOpen(false)}>
            Отмена
          </button>
        </div>
      )}
    </div>
  )
}
