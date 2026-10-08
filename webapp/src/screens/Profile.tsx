import { useEffect, useState } from 'react'
import { useNav, useSessionReload } from '../context'
import { useCategories } from '../lib/categories'
import { ApiError, getProfile, patchProfile } from '../lib/api'
import { haptic, toast } from '../lib/telegram'
import type { Profile as ProfileData, ProfilePatch, SalaryMode } from '../lib/types'
import { useAsync } from '../hooks'
import { ErrorBanner } from '../components/ErrorBanner'
import { Loader } from '../components/Loader'

/** Editable value: the typed text, or `reset` to go back to the shared default. */
interface Draft {
  text: string
  reset: boolean
}

// Salary and advance amounts are not set here: payments are recorded as income with the amount
// that actually arrived. The profile keeps only the schedule (one or two payments) and reminders.
interface Form {
  displayName: Draft
  salaryMode: SalaryMode
  salaryReminders: boolean
}

function toForm(p: ProfileData): Form {
  return {
    displayName: { text: p.displayName.value, reset: false },
    salaryMode: p.salaryMode,
    salaryReminders: p.salaryReminders,
  }
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
  // Build the patch: only changed fields; null resets to default.
  const patch: ProfilePatch = {}
  if (form.displayName.reset) {
    if (!p.displayName.isDefault) patch.displayName = null
  } else if (name !== p.displayName.value) patch.displayName = name
  const split = form.salaryMode === 'split'
  if (form.salaryMode !== p.salaryMode) patch.salaryMode = form.salaryMode
  if (form.salaryReminders !== p.salaryReminders) patch.salaryReminders = form.salaryReminders

  const hasErrors = !!nameError
  const dirty = Object.keys(patch).length > 0
  const canSave = dirty && !hasErrors && !saving

  function setDraft(key: 'displayName', text: string) {
    setForm((f) => f && { ...f, [key]: { text, reset: false } })
  }
  function resetDraft(key: 'displayName') {
    setForm((f) => f && { ...f, [key]: { text: p!.displayName.default, reset: true } })
  }

  async function save() {
    if (!canSave) return
    setSaving(true)
    setError(null)
    try {
      const updated = await patchProfile(patch)
      profile.reload()
      setForm(toForm(updated))
      reloadSession() // effective name everywhere (FR-017)
      haptic('success')
      toast('Сохранено')
    } catch (e) {
      haptic('error')
      setError(e)
    } finally {
      setSaving(false)
    }
  }

  const showNameReset = !form.displayName.reset && (!p.displayName.isDefault || name !== p.displayName.default)

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
        <div className="hint">
          {split
            ? 'Аванс — около 15 числа, остаток — в последний день месяца. В день выплаты запишите, сколько пришло: суммы берутся из прихода.'
            : 'Зарплата — в последний день месяца. В день выплаты запишите, сколько пришло: сумма берётся из прихода.'}
        </div>
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
      {serverField && !['displayName', 'salaryMode'].includes(serverField) && (
        <ErrorBanner error={error} />
      )}

      <button className="btn" disabled={!canSave} onClick={save}>
        {saving ? 'Сохраняю…' : 'Сохранить'}
      </button>
    </div>
  )
}
