import { useRef, useState } from 'react'
import { useSession } from '../context'
import { addGoal, ApiError, deleteGoal, listGoals, newClientId, patchGoal } from '../lib/api'
import { diramToInput, formatSomoni, parseAmountInput } from '../lib/money'
import { allowedQuarters } from '../lib/months'
import { confirmAction, haptic, toast } from '../lib/telegram'
import type { Goal, GoalPatch } from '../lib/types'
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
  const active = g?.items.filter((x) => x.status !== 'done') ?? []
  const done = g?.items.filter((x) => x.status === 'done') ?? []
  const [conflict, setConflict] = useState(false)
  function onConflict() {
    setConflict(true)
    goals.reload()
  }

  return (
    <div className="screen">
      <div className="screen-title">🎯 Цели на квартал</div>
      {goals.loading && !g && <Loader />}
      {goals.error != null && <ErrorBanner error={goals.error} onRetry={goals.reload} />}

      {conflict && (
        <div className="banner" role="alert">
          ⚠️ Цель изменили на другом устройстве — показаны актуальные данные.
        </div>
      )}
      {g && g.items.length === 0 && <div className="card hint">Целей ещё нет. Добавь первую!</div>}
      {g && active.length > 0 && <div className="card-title">Активные</div>}
      {active.map((goal) => (
        <GoalCard
          key={goal.id + ':' + goal.version}
          goal={goal}
          savingsBalance={g!.savingsBalance}
          quarters={quarters}
          onChanged={goals.reload}
          onConflict={onConflict}
        />
      ))}
      {g && done.length > 0 && <div className="card-title">Выполненные</div>}
      {done.map((goal) => (
        <GoalCard
          key={goal.id + ':' + goal.version}
          goal={goal}
          savingsBalance={g!.savingsBalance}
          quarters={quarters}
          onChanged={goals.reload}
          onConflict={onConflict}
        />
      ))}

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

interface CardProps {
  goal: Goal
  savingsBalance: number
  quarters: string[]
  onChanged(): void
  onConflict(): void
}

/** One goal: progress, status toggle, inline edit, delete (FR-013). */
function GoalCard({ goal, savingsBalance, quarters, onChanged, onConflict }: CardProps) {
  const [editing, setEditing] = useState(false)
  const [name, setName] = useState(goal.name)
  const [target, setTarget] = useState(diramToInput(goal.target))
  const [quarter, setQuarter] = useState(goal.quarter)
  const [note, setNote] = useState(goal.note ?? '')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<unknown>(null)
  // One requestId per attempt; reused on retry of the same attempt (idempotent PATCH).
  const requestId = useRef<string | null>(null)

  const pct = Math.round(goal.progress * 100)
  const parsed = parseAmountInput(target)
  const field = error instanceof ApiError && error.code === 'validation' ? error.field : undefined
  const nameErr = !name.trim() ? 'Введите название' : field === 'name' ? (error as ApiError).message : null
  const targetErr = !parsed.ok ? parsed.error : field === 'target' ? (error as ApiError).message : null
  // The stored quarter stays selectable even if it is now in the past.
  const quarterOptions = quarters.includes(goal.quarter) ? quarters : [goal.quarter, ...quarters]

  async function send(body: Omit<GoalPatch, 'version' | 'requestId'>) {
    setBusy(true)
    setError(null)
    if (!requestId.current) requestId.current = newClientId()
    try {
      await patchGoal(goal.id, { version: goal.version, requestId: requestId.current, ...body })
      requestId.current = null
      haptic('success')
      setEditing(false)
      onChanged()
    } catch (e) {
      haptic('error')
      if (e instanceof ApiError && e.status !== 0 && e.status < 500) requestId.current = null
      if (e instanceof ApiError && e.code === 'conflict') onConflict()
      else setError(e)
    } finally {
      setBusy(false)
    }
  }

  function saveEdit() {
    if (nameErr || !parsed.ok) return
    const body: Omit<GoalPatch, 'version' | 'requestId'> = {}
    if (name.trim() !== goal.name) body.name = name.trim()
    if (parsed.diram !== goal.target) body.target = parsed.diram
    if (quarter !== goal.quarter) body.quarter = quarter
    if (note.trim() !== (goal.note ?? '')) body.note = note.trim()
    if (Object.keys(body).length === 0) {
      setEditing(false)
      return
    }
    void send(body)
  }

  async function remove() {
    if (!(await confirmAction(`Удалить цель «${goal.name}»? Накопления не изменятся.`))) return
    setBusy(true)
    setError(null)
    try {
      await deleteGoal(goal.id, goal.version)
      haptic('success')
      toast('Цель удалена')
      onChanged()
    } catch (e) {
      haptic('error')
      if (e instanceof ApiError && e.code === 'conflict') onConflict()
      else setError(e)
    } finally {
      setBusy(false)
    }
  }

  if (editing) {
    return (
      <div className="card">
        <div className="card-title">✏️ Цель</div>
        <div className="field">
          <label htmlFor={'gn-' + goal.id}>Название</label>
          <input
            id={'gn-' + goal.id}
            className={'input' + (nameErr ? ' invalid' : '')}
            maxLength={60}
            value={name}
            onChange={(e) => {
              requestId.current = null
              setName(e.target.value)
            }}
          />
          {nameErr && <div className="error-text">{nameErr}</div>}
        </div>
        <AmountInput
          label="Целевая сумма, с."
          autoFocus={false}
          value={target}
          onChange={(v) => {
            requestId.current = null
            setTarget(v)
          }}
          error={targetErr}
        />
        <div className="field">
          <label>Квартал</label>
          <div className="chips">
            {quarterOptions.map((q) => (
              <button
                key={q}
                type="button"
                className={'chip' + (q === quarter ? ' selected' : '')}
                onClick={() => {
                  requestId.current = null
                  setQuarter(q)
                }}
              >
                {q}
              </button>
            ))}
          </div>
          {field === 'quarter' && <div className="error-text">{(error as ApiError).message}</div>}
        </div>
        <div className="field">
          <label htmlFor={'gd-' + goal.id}>Описание</label>
          <input
            id={'gd-' + goal.id}
            className="input"
            maxLength={200}
            value={note}
            onChange={(e) => {
              requestId.current = null
              setNote(e.target.value)
            }}
          />
        </div>
        {error != null && !field && <ErrorBanner error={error} onRetry={saveEdit} />}
        <button className="btn" disabled={busy || !!nameErr || !parsed.ok} onClick={saveEdit}>
          {busy ? 'Сохраняю…' : 'Сохранить'}
        </button>
        <button
          className="btn-link"
          onClick={() => {
            setEditing(false)
            setError(null)
            setName(goal.name)
            setTarget(diramToInput(goal.target))
            setQuarter(goal.quarter)
            setNote(goal.note ?? '')
          }}
        >
          Отмена
        </button>
      </div>
    )
  }

  return (
    <div className="card">
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
      <div className="hint">💎 из накоплений {formatSomoni(savingsBalance)}</div>
      {goal.note && <div className="hint">📝 {goal.note}</div>}
      {error != null && <ErrorBanner error={error} />}
      <div className="goal-actions">
        <button
          className="chip"
          disabled={busy}
          onClick={() => void send({ status: goal.status === 'done' ? 'active' : 'done' })}
        >
          {goal.status === 'done' ? '↺ Вернуть' : '✓ Выполнена'}
        </button>
        <button className="chip" disabled={busy} onClick={() => setEditing(true)}>
          ✏️ Изменить
        </button>
        <button className="chip" disabled={busy} onClick={() => void remove()}>
          🗑 Удалить
        </button>
      </div>
    </div>
  )
}
