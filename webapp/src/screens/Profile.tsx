import { useEffect, useState } from 'react'
import { useNav, useSessionReload } from '../context'
import { useCategories } from '../lib/categories'
import { ApiError, getProfile, patchProfile } from '../lib/api'
import { diramToInput, formatSomoni, parseAmountInput } from '../lib/money'
import { haptic, toast } from '../lib/telegram'
import type { Profile as ProfileData, ProfilePatch } from '../lib/types'
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
}

function toForm(p: ProfileData): Form {
  return {
    displayName: { text: p.displayName.value, reset: false },
    salary: { text: diramToInput(p.salary.value), reset: false },
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

  const hasErrors = !!nameError || !!salaryError
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
      {serverField && !['displayName', 'salary'].includes(serverField) && (
        <ErrorBanner error={error} />
      )}

      <button className="btn" disabled={!canSave} onClick={save}>
        {saving ? 'Сохраняю…' : 'Сохранить'}
      </button>
    </div>
  )
}
