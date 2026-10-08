import { useRef, useState } from 'react'
import { useNav, useSession, type AddKind } from '../context'
import { addTransaction, ApiError, newClientId } from '../lib/api'
import { formatSomoni, parseAmountInput } from '../lib/money'
import { haptic } from '../lib/telegram'
import type { WriteResult } from '../lib/types'
import { AmountInput } from '../components/AmountInput'
import { CategoryGrid } from '../components/CategoryGrid'
import { ErrorBanner } from '../components/ErrorBanner'

const TITLES: Record<AddKind, string> = {
  expense: '➕ Расход',
  income: '💵 Приход',
  credit_purchase: '💳 Расход по карте',
}

export function AddTransaction({ kind }: { kind: AddKind }) {
  const nav = useNav()
  const { categories } = useSession()
  const needsCategory = kind !== 'income'

  const [amount, setAmount] = useState('')
  const [note, setNote] = useState('')
  const [category, setCategory] = useState<string | null>(null)
  const [touched, setTouched] = useState(false)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<unknown>(null)
  const [result, setResult] = useState<WriteResult | null>(null)
  // One id per form instance, reused on retry so a resubmit never duplicates (FR-010).
  const clientId = useRef(newClientId())

  const parsed = parseAmountInput(amount)
  const amountError = touched && !parsed.ok ? parsed.error : null
  const serverFieldError = error instanceof ApiError && error.code === 'validation' ? error.field : undefined
  const canSave = parsed.ok && (!needsCategory || category !== null) && !saving

  async function save() {
    setTouched(true)
    if (!parsed.ok || (needsCategory && !category)) return
    setSaving(true)
    setError(null)
    try {
      const res = await addTransaction({
        clientId: clientId.current,
        kind,
        amount: parsed.diram,
        category: needsCategory ? category! : undefined,
        note: note.trim() || undefined,
      })
      haptic('success')
      setResult(res)
    } catch (e) {
      haptic('error')
      setError(e)
    } finally {
      setSaving(false)
    }
  }

  function again() {
    clientId.current = newClientId()
    setAmount('')
    setNote('')
    setCategory(null)
    setTouched(false)
    setError(null)
    setResult(null)
  }

  if (result) {
    const t = result.transaction
    const s = result.summary
    const cat = categories.find((c) => c.name === t.category)
    const line = s.categories.find((c) => c.name === t.category)
    return (
      <div className="screen">
        <div className="card success">
          <div className="card-title">✅ Записано</div>
          <div className="big-number">{formatSomoni(t.amount)}</div>
          {cat && <div>{cat.label}</div>}
          {t.note && <div className="hint">{t.note}</div>}
          <div className="row">
            <span>💚 Остаток</span>
            <span>{formatSomoni(s.remaining)}</span>
          </div>
          {kind === 'expense' && line && line.limit !== null && (
            <div className="row">
              <span>{line.label}</span>
              <span>
                {formatSomoni(line.spent)} / {formatSomoni(line.limit)} (
                {Math.round((line.spent / line.limit) * 100)}%)
              </span>
            </div>
          )}
          {kind === 'credit_purchase' && (
            <div className="row">
              <span>💳 Долг по карте</span>
              <span>{formatSomoni(s.creditDebt)}</span>
            </div>
          )}
        </div>
        <button className="btn" onClick={again}>
          Ещё запись
        </button>
        <button className="btn btn-secondary" onClick={nav.pop}>
          На главную
        </button>
      </div>
    )
  }

  return (
    <div className="screen">
      <div className="screen-title">{TITLES[kind]}</div>
      <div className="card">
        <AmountInput
          value={amount}
          onChange={(v) => {
            setAmount(v)
            setTouched(true)
          }}
          error={amountError ?? (serverFieldError === 'amount' ? (error as ApiError).message : null)}
        />
        <div className="field">
          <label htmlFor="note">Описание (необязательно)</label>
          <input
            id="note"
            className="input"
            maxLength={200}
            placeholder="например, такси"
            value={note}
            onChange={(e) => setNote(e.target.value)}
          />
        </div>
      </div>
      {needsCategory && (
        <div className="card">
          <CategoryGrid selected={category} onSelect={setCategory} />
          {touched && !category && <div className="error-text">Выберите категорию</div>}
        </div>
      )}
      {error != null && serverFieldError !== 'amount' && <ErrorBanner error={error} onRetry={canSave ? save : undefined} />}
      <button className="btn" disabled={!canSave} onClick={save}>
        {saving ? 'Сохраняю…' : 'Сохранить'}
      </button>
    </div>
  )
}
