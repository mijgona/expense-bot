import { useRef, useState } from 'react'
import { useNav, type AddKind } from '../context'
import { addTransaction, ApiError, newClientId } from '../lib/api'
import { useCategories } from '../lib/categories'
import { formatSomoni, parseAmountInput } from '../lib/money'
import { todayDushanbe } from '../lib/months'
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
  const { byId } = useCategories()
  const needsCategory = kind !== 'income'

  const [amount, setAmount] = useState('')
  const [note, setNote] = useState('')
  const [category, setCategory] = useState<string | null>(null)
  const today = todayDushanbe()
  const [date, setDate] = useState(today)
  const [touched, setTouched] = useState(false)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<unknown>(null)
  const [result, setResult] = useState<WriteResult | null>(null)
  // One id per form instance, reused on retry so a resubmit never duplicates (FR-010).
  const clientId = useRef(newClientId())

  const parsed = parseAmountInput(amount)
  const amountError = touched && !parsed.ok ? parsed.error : null
  const serverFieldError = error instanceof ApiError && error.code === 'validation' ? error.field : undefined
  // 422 balance rule (savings / card debt would go negative at some date) is shown under the amount.
  const balanceError = error instanceof ApiError && error.status === 422 ? error.message : null
  const dateError = date > today ? 'Дата не может быть в будущем' : null
  const canSave = parsed.ok && (!needsCategory || category !== null) && !dateError && !saving

  async function save() {
    setTouched(true)
    if (!parsed.ok || (needsCategory && !category) || dateError) return
    setSaving(true)
    setError(null)
    try {
      const res = await addTransaction({
        clientId: clientId.current,
        kind,
        amount: parsed.diram,
        category: needsCategory ? category! : undefined,
        note: note.trim() || undefined,
        date: date && date !== today ? date : undefined,
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
    setDate(today)
    setTouched(false)
    setError(null)
    setResult(null)
  }

  if (result) {
    const t = result.transaction
    const s = result.summary
    const cat = byId(t.category)
    const line = s.categories.find((c) => c.id === t.category)
    return (
      <div className="screen">
        <div className="card success">
          <div className="card-title">✅ Записано</div>
          <div className="big-number">{formatSomoni(t.amount)}</div>
          {cat && <div>{cat.name}</div>}
          {t.note && <div className="hint">{t.note}</div>}
          <div className="row">
            <span>💚 Остаток</span>
            <span>{formatSomoni(s.remaining)}</span>
          </div>
          {kind === 'expense' && line && line.limit !== null && line.limit > 0 && (
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
          error={amountError ?? (serverFieldError === 'amount' ? (error as ApiError).message : balanceError)}
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
        <div className="field">
          <label htmlFor="date">Дата</label>
          <input
            id="date"
            type="date"
            className={'input' + (dateError || serverFieldError === 'date' ? ' invalid' : '')}
            max={today}
            value={date}
            onChange={(e) => setDate(e.target.value || today)}
          />
          {(dateError || serverFieldError === 'date') && (
            <div className="error-text">{dateError ?? (error as ApiError).message}</div>
          )}
        </div>
      </div>
      {needsCategory && (
        <div className="card">
          <CategoryGrid selected={category} onSelect={setCategory} />
          {touched && !category && <div className="error-text">Выберите категорию</div>}
        </div>
      )}
      {error != null && serverFieldError !== 'amount' && serverFieldError !== 'date' && !balanceError && <ErrorBanner error={error} onRetry={canSave ? save : undefined} />}
      <button className="btn" disabled={!canSave} onClick={save}>
        {saving ? 'Сохраняю…' : 'Сохранить'}
      </button>
    </div>
  )
}
