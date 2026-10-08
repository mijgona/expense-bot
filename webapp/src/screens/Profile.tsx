import { useEffect, useState } from 'react'
import { useNav, useSessionReload } from '../context'
import { useCategories } from '../lib/categories'
import { ApiError, getProfile, patchProfile } from '../lib/api'
import { diramToInput, formatSomoni, parseAmountInput } from '../lib/money'
import { haptic, toast } from '../lib/telegram'
import type { Profile as ProfileData, ProfilePatch, SalaryMode } from '../lib/types'
import { useAsync } from '../hooks'
import { AmountInput } from '../components/AmountInput'
import { ErrorBanner } from '../components/ErrorBanner'
import { Loader } from '../components/Loader'

/** Editable value: the typed text, or `reset` to go back to the shared default. */
interface Draft {
  text: string
  reset: boolean
}

interface Form {
  displayName: Draft
  salary: Draft
  salaryMode: SalaryMode
  /** touched = the user typed or reset it; an untouched advance is never sent. */
  advance: Draft & { touched: boolean }
  salaryReminders: boolean
}

function toForm(p: ProfileData): Form {
  return {
    displayName: { text: p.displayName.value, reset: false },
    salary: { text: diramToInput(p.salary.value), reset: false },
    salaryMode: p.salaryMode,
    advance: { text: diramToInput(p.advance.value), reset: false, touched: false },
    salaryReminders: p.salaryReminders,
  }
}

/** Default advance: half the salary, rounded down to whole somoni (mirrors payroll.EffectiveAdvance). */
function halfSalary(salary: number): number {
  return Math.floor(salary / 2 / 100) * 100
}

export function Profile() {
  const nav = useNav()
  const cats = useCategories()
  const reloadSession = useSessionReload()
  const profile = useAsync(() => getProfile(), [])
  const [form, setForm] = useState<Form | null>(null)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<unknown>(null)

  useEffect(() => {
    if (profile.data) setForm(toForm(profile.data))
  }, [profile.data])

  const p = profile.data
  if (!p || !form) {
    return (
      <div className="screen">
        <div className="screen-title">👤 Профиль</div>
        {profile.error != null ? <ErrorBanner error={profile.error} onRetry={profile.reload} /> : <Loader />}
      </div>
    )
  }

  const apiErr = error instanceof ApiError ? error : null
  const serverField = apiErr?.code === 'validation' ? apiErr.field : undefined

  // Client-side validation.
  const name = form.displayName.text.trim()
  const nameError = form.displayName.reset
    ? null
    : name.length < 1
      ? 'Введите имя'
      : [...name].length > 40
        ? 'Не длиннее 40 символов'
        : null
  const salaryParsed = parseAmountInput(form.salary.text)
  const salaryError = form.salary.reset ? null : !salaryParsed.ok ? salaryParsed.error : null

  // Build the patch: only changed fields; null resets to default.
  const patch: ProfilePatch = {}
  if (form.displayName.reset) {
    if (!p.displayName.isDefault) patch.displayName = null
  } else if (name !== p.displayName.value) patch.displayName = name
  if (form.salary.reset) {
    if (!p.salary.isDefault) patch.salary = null
  } else if (salaryParsed.ok && salaryParsed.diram !== p.salary.value) patch.salary = salaryParsed.diram

  // Salary schedule (007). The rest is derived live from what is typed.
  const salaryNow = form.salary.reset ? p.salary.default : salaryParsed.ok ? salaryParsed.diram : p.salary.value
  const split = form.salaryMode === 'split'
  const advanceParsed = parseAmountInput(form.advance.text)
  const advanceNow = form.advance.reset
    ? halfSalary(salaryNow)
    : form.advance.touched
      ? advanceParsed.ok
        ? advanceParsed.diram
        : null
      : p.advance.isDefault
        ? halfSalary(salaryNow)
        : p.advance.value
  const advanceError =
    !split || form.advance.reset || !form.advance.touched
      ? null
      : !advanceParsed.ok
        ? advanceParsed.error
        : advanceParsed.diram < 100 || advanceParsed.diram > salaryNow - 100
          ? `Аванс: от 1 с. до ${formatSomoni(Math.max(salaryNow - 100, 100))}`
          : null
  const restNow = advanceNow != null ? salaryNow - advanceNow : null

  if (form.salaryMode !== p.salaryMode) patch.salaryMode = form.salaryMode
  if (split) {
    if (form.advance.reset) {
      if (!p.advance.isDefault) patch.advance = null
    } else if (form.advance.touched && advanceParsed.ok && (p.advance.isDefault || advanceParsed.diram !== p.advance.value)) {
      patch.advance = advanceParsed.diram
    }
  }
  if (form.salaryReminders !== p.salaryReminders) patch.salaryReminders = form.salaryReminders

  const hasErrors = !!nameError || !!salaryError || !!advanceError
  const dirty = Object.keys(patch).length > 0
  const canSave = dirty && !hasErrors && !saving

  function setDraft(key: 'displayName' | 'salary', text: string) {
    setForm((f) => f && { ...f, [key]: { text, reset: false } })
  }
  function resetDraft(key: 'displayName' | 'salary') {
    setForm(
      (f) =>
        f && {
          ...f,
          [key]: {
            text: key === 'salary' ? diramToInput(p!.salary.default) : p!.displayName.default,
            reset: true,
          },
        },
    )
  }

  async function save() {
    if (!canSave) return
    setSaving(true)
    setError(null)
    try {
      const updated = await patchProfile(patch)
      profile.reload()
      setForm(toForm(updated))
      reloadSession() // effective salary / name everywhere (FR-017)
      haptic('success')
      toast('Сохранено')
    } catch (e) {
      haptic('error')
      setError(e)
    } finally {
      setSaving(false)
    }
  }

  function setAdvance(text: string) {
    setForm((f) => f && { ...f, advance: { text, reset: false, touched: true } })
  }
  function resetAdvance() {
    setForm((f) => f && { ...f, advance: { text: diramToInput(halfSalary(salaryNow)), reset: true, touched: true } })
  }
  const showAdvanceReset = !form.advance.reset && (!p.advance.isDefault || form.advance.touched)

  const showNameReset = !form.displayName.reset && (!p.displayName.isDefault || name !== p.displayName.default)
  const showSalaryReset =
    !form.salary.reset && (!p.salary.isDefault || (salaryParsed.ok && salaryParsed.diram !== p.salary.default))

  return (
    <div className="screen">
      <div className="screen-title">👤 Профиль</div>
      <div className="card">
        <div className="row">
          <span className="hint">Telegram</span>
          <span>
            {p.telegram.firstName}
            {p.telegram.username ? ` · @${p.telegram.username}` : ''}
          </span>
        </div>
      </div>

      <div className="card">
        <div className="field">
          <label htmlFor="dname">Имя в приложении</label>
          <input
            id="dname"
            className={'input' + (nameError || serverField === 'displayName' ? ' invalid' : '')}
            maxLength={40}
            value={form.displayName.text}
            onChange={(e) => setDraft('displayName', e.target.value)}
          />
          {(nameError || serverField === 'displayName') && (
            <div className="error-text">{nameError ?? apiErr!.message}</div>
          )}
          {showNameReset && (
            <button className="btn-link" onClick={() => resetDraft('displayName')}>
              Сбросить ({p.displayName.default})
            </button>
          )}
        </div>
        <AmountInput
          label="Зарплата в месяц, с."
          autoFocus={false}
          value={form.salary.text}
          onChange={(v) => setDraft('salary', v)}
          error={salaryError ?? (serverField === 'salary' ? apiErr!.message : null)}
        />
        {showSalaryReset && (
          <button className="btn-link" onClick={() => resetDraft('salary')}>
            Сбросить ({formatSomoni(p.salary.default)})
          </button>
        )}
      </div>

      <div className="card">
        <div className="card-title">💼 Зарплата</div>
        <div className="segmented">
          <button
            className={form.salaryMode === 'single' ? 'active' : ''}
            onClick={() => setForm((f) => f && { ...f, salaryMode: 'single' })}
          >
            Один раз в месяц
          </button>
          <button
            className={form.salaryMode === 'split' ? 'active' : ''}
            onClick={() => setForm((f) => f && { ...f, salaryMode: 'split' })}
          >
            Два раза: аванс и остаток
          </button>
        </div>
        {serverField === 'salaryMode' && <div className="error-text">{apiErr!.message}</div>}
        {split ? (
          <>
            <AmountInput
              id="advance"
              label="Аванс (15 числа), с."
              autoFocus={false}
              value={form.advance.reset || form.advance.touched ? form.advance.text : diramToInput(advanceNow ?? 0)}
              onChange={setAdvance}
              error={advanceError ?? (serverField === 'advance' ? apiErr!.message : null)}
            />
            {showAdvanceReset && (
              <button className="btn-link" onClick={resetAdvance}>
                Сбросить (половина — {formatSomoni(halfSalary(salaryNow))})
              </button>
            )}
            <div className="row">
              <span className="hint">Остаток (последний день месяца)</span>
              <span>{restNow != null ? formatSomoni(restNow) : '—'}</span>
            </div>
          </>
        ) : (
          <div className="hint">Вся зарплата — в последний день месяца.</div>
        )}
        <label className="check">
          <input
            type="checkbox"
            checked={form.salaryReminders}
            onChange={(e) => setForm((f) => f && { ...f, salaryReminders: e.target.checked })}
          />
          <span>Напоминать о зарплате в Telegram</span>
        </label>
      </div>

      <div className="card clickable" onClick={() => nav.push({ name: 'categories' })}>
        <div className="row">
          <span>🏷 Категории</span>
          <span className="hint">
            {cats.visible.length}
            {cats.all.length > cats.visible.length ? ` + ${cats.all.length - cats.visible.length} скрытых` : ''} ›
          </span>
        </div>
        <div className="hint">Добавить, переименовать, скрыть, лимиты и порядок</div>
      </div>

      {error != null && !serverField && <ErrorBanner error={error} onRetry={canSave ? save : undefined} />}
      {serverField && !['displayName', 'salary', 'advance', 'salaryMode'].includes(serverField) && (
        <ErrorBanner error={error} />
      )}

      <button className="btn" disabled={!canSave} onClick={save}>
        {saving ? 'Сохраняю…' : 'Сохранить'}
      </button>
    </div>
  )
}
