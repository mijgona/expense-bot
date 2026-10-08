import { useRef, useState } from 'react'
import { useNav } from '../context'
import { useCategories } from '../lib/categories'
import { ApiError, deleteTransaction, listGoals, newClientId, patchTransaction } from '../lib/api'
import { diramToInput, formatSomoni, parseAmountInput } from '../lib/money'
import { formatDate, toDateInput, todayDushanbe } from '../lib/months'
import { confirmAction, haptic, toast } from '../lib/telegram'
import type { Transaction, TransactionPatch } from '../lib/types'
import { AmountInput } from '../components/AmountInput'
import { CategoryGrid } from '../components/CategoryGrid'
import { ErrorBanner } from '../components/ErrorBanner'
import { GoalPicker } from '../components/GoalPicker'
import { useAsync } from '../hooks'
import { KIND_ICON, KIND_LABEL } from '../components/TransactionRow'

interface FormValues {
  amount: string
  category: string | null
  note: string
  date: string
  /** savings_deposit only; '' = no goal. */
  goalId: string
}

function valuesOf(tx: Transaction): FormValues {
  return {
    amount: diramToInput(tx.amount),
    category: tx.category ?? null,
    note: tx.note ?? '',
    date: toDateInput(tx.occurredAt ?? tx.createdAt),
    goalId: tx.goalId ?? '',
  }
}

export function EditTransaction({ tx: initial }: { tx: Transaction }) {
  const nav = useNav()
  const { byId, labelOf } = useCategories()
  const today = todayDushanbe()

  // `base` is the server's version of the record the form is editing (replaced on 409).
  const [base, setBase] = useState<Transaction>(initial)
  const [form, setForm] = useState<FormValues>(() => valuesOf(initial))
  const [mine, setMine] = useState<FormValues | null>(null) // user's values before a conflict reload
  const [saving, setSaving] = useState(false)
  const [deleting, setDeleting] = useState(false)
  const [error, setError] = useState<unknown>(null)
  // One requestId per save attempt, reused on retry until it succeeds (FR-010).
  const requestId = useRef<string | null>(null)

  const needsCategory = base.kind === 'expense' || base.kind === 'credit_purchase'
  const isDeposit = base.kind === 'savings_deposit'
  const goals = useAsync(() => (isDeposit ? listGoals() : Promise.resolve(null)), [isDeposit])
  const orig = valuesOf(base)
  // Active goals, plus the deposit's current goal even if it is already done.
  const goalOptions = goals.data?.items.filter((g) => g.status !== 'done' || g.id === orig.goalId) ?? []
  const parsed = parseAmountInput(form.amount)
  const amountError = !parsed.ok ? parsed.error : null
  const dateError = form.date > today ? 'Дата не может быть в будущем' : !form.date ? 'Укажите дату' : null
  const selectedCat = byId(form.category)
  const categoryValid = !needsCategory || (selectedCat !== undefined && !selectedCat.hidden)
  // A record in a hidden (or unknown) category stays valid as long as the category is not changed (FR-013).
  const categoryOk = categoryValid || form.category === orig.category
  const origCat = byId(orig.category)
  const origHidden = needsCategory && orig.category != null && (!origCat || origCat.hidden)
  // For a record in a hidden category the picker opens only on «Изменить».
  const [pickCategory, setPickCategory] = useState(false)

  const changed =
    (parsed.ok && parsed.diram !== base.amount) ||
    form.category !== orig.category ||
    form.note.trim() !== orig.note.trim() ||
    form.date !== orig.date ||
    form.goalId !== orig.goalId
  const canSave = changed && parsed.ok && !dateError && categoryOk && !saving && !deleting

  const apiErr = error instanceof ApiError ? error : null
  const field = apiErr?.code === 'validation' ? apiErr.field : undefined
  const balanceMsg = apiErr?.status === 422 ? apiErr.message : null
  const conflict = apiErr?.code === 'conflict'

  function update(patch: Partial<FormValues>) {
    // Different values = a different request; only an unchanged retry may reuse the id.
    requestId.current = null
    setForm((f) => ({ ...f, ...patch }))
  }

  function handleConflict(e: ApiError) {
    const current = e.current as Transaction | undefined
    if (current && typeof current === 'object' && 'version' in current) {
      setMine(form)
      setBase(current)
      setForm(valuesOf(current))
      requestId.current = null
    }
  }

  async function save() {
    if (!canSave || !parsed.ok) return
    setSaving(true)
    setError(null)
    if (!requestId.current) requestId.current = newClientId()
    const body: TransactionPatch = { version: base.version, requestId: requestId.current }
    if (parsed.diram !== base.amount) body.amount = parsed.diram
    if (needsCategory && form.category !== orig.category && form.category) body.category = form.category
    if (form.note.trim() !== orig.note.trim()) body.note = form.note.trim()
    if (form.date !== orig.date) body.date = form.date
    if (isDeposit && form.goalId !== orig.goalId) body.goalId = form.goalId
    try {
      await patchTransaction(base.id, body)
      requestId.current = null
      haptic('success')
      toast('Сохранено')
      nav.pop()
    } catch (e) {
      haptic('error')
      setError(e)
      if (e instanceof ApiError) {
        // A definitive rejection means this attempt is over; the next save is a new request.
        if (e.status === 400 || e.status === 409 || e.status === 422) requestId.current = null
        if (e.code === 'conflict') handleConflict(e)
      }
    } finally {
      setSaving(false)
    }
  }

  async function remove() {
    const ok = await confirmAction('Удалить запись? Это нельзя отменить.')
    if (!ok) return
    setDeleting(true)
    setError(null)
    try {
      await deleteTransaction(base.id, base.version) // 204 (already gone) also counts as success
      haptic('success')
      toast('Удалено')
      nav.pop()
    } catch (e) {
      haptic('error')
      setError(e)
      if (e instanceof ApiError && e.code === 'conflict') handleConflict(e)
    } finally {
      setDeleting(false)
    }
  }

  return (
    <div className="screen">
      <div className="screen-title">✏️ Редактирование</div>
      <div className="hint">
        {KIND_ICON[base.kind]} {KIND_LABEL[base.kind]} · тип менять нельзя
        {base.editedAt ? ` · изменено ${formatDate(base.editedAt)}` : ''}
      </div>

      {conflict && (
        <div className="banner" role="alert">
          <div>⚠️ Запись изменили на другом устройстве. Показаны актуальные значения — проверьте и сохраните ещё раз.</div>
          {mine && (
            <div className="hint">
              Ваш вариант: {mine.amount} с.{mine.category ? ` · ${labelOf(mine.category)}` : ''}
              {mine.note ? ` · ${mine.note}` : ''} · {mine.date.split('-').reverse().join('.')}
            </div>
          )}
        </div>
      )}

      <div className="card">
        <AmountInput
          value={form.amount}
          autoFocus={false}
          onChange={(v) => update({ amount: v })}
          error={amountError ?? (field === 'amount' ? apiErr!.message : balanceMsg)}
        />
        <div className="field">
          <label htmlFor="note">Описание</label>
          <input
            id="note"
            className="input"
            maxLength={200}
            value={form.note}
            onChange={(e) => update({ note: e.target.value })}
          />
          {field === 'note' && <div className="error-text">{apiErr!.message}</div>}
        </div>
        <div className="field">
          <label htmlFor="date">Дата</label>
          <input
            id="date"
            type="date"
            className={'input' + (dateError || field === 'date' ? ' invalid' : '')}
            max={today}
            value={form.date}
            onChange={(e) => update({ date: e.target.value })}
          />
          {(dateError || field === 'date') && <div className="error-text">{dateError ?? apiErr!.message}</div>}
        </div>
      </div>

      {needsCategory && (
        <div className="card">
          {origHidden && !pickCategory ? (
            <div className="row">
              <span>
                Категория: {labelOf(orig.category)} <span className="tag">скрыта</span>
              </span>
              <button type="button" className="btn-link" onClick={() => setPickCategory(true)}>
                Изменить
              </button>
            </div>
          ) : (
            <CategoryGrid selected={form.category} onSelect={(c) => update({ category: c })} />
          )}
          {field === 'category' && <div className="error-text">{apiErr!.message}</div>}
        </div>
      )}

      {isDeposit && goalOptions.length > 0 && (
        <div className="card">
          <GoalPicker
            goals={goalOptions}
            selected={form.goalId}
            onSelect={(id) => update({ goalId: id })}
            error={field === 'goalId' ? apiErr!.message : null}
          />
        </div>
      )}

      {error != null && !conflict && !field && !balanceMsg && <ErrorBanner error={error} onRetry={canSave ? save : undefined} />}

      <button className="btn" disabled={!canSave} onClick={save}>
        {saving ? 'Сохраняю…' : 'Сохранить'}
      </button>
      <button className="btn btn-danger" disabled={saving || deleting} onClick={remove}>
        {deleting ? 'Удаляю…' : '🗑 Удалить'}
      </button>
      <div className="hint">Сумма записи сейчас: {formatSomoni(base.amount)}</div>
    </div>
  )
}
