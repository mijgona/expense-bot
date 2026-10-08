import { useEffect, useState } from 'react'
import { useSessionReload } from '../context'
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
  limits: Record<string, Draft>
}

function toForm(p: ProfileData): Form {
  const limits: Record<string, Draft> = {}
  for (const l of p.limits) limits[l.name] = { text: l.value === 0 ? '' : diramToInput(l.value), reset: false }
  return {
    displayName: { text: p.displayName.value, reset: false },
    salary: { text: diramToInput(p.salary.value), reset: false },
    limits,
  }
}

/** Limit input: empty or 0 means "no limit" (0). */
function parseLimit(text: string): { ok: true; diram: number } | { ok: false; error: string } {
  const t = text.trim()
  if (t === '' || /^0+([.,]0*)?$/.test(t)) return { ok: true, diram: 0 }
  return parseAmountInput(t)
}

export function Profile() {
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
  const limitErrors: Record<string, string | null> = {}
  for (const l of p.limits) {
    const d = form.limits[l.name]
    const r = parseLimit(d.text)
    limitErrors[l.name] = d.reset || r.ok ? null : r.error
  }

  // Build the patch: only changed fields; null resets to default.
  const patch: ProfilePatch = {}
  if (form.displayName.reset) {
    if (!p.displayName.isDefault) patch.displayName = null
  } else if (name !== p.displayName.value) patch.displayName = name
  if (form.salary.reset) {
    if (!p.salary.isDefault) patch.salary = null
  } else if (salaryParsed.ok && salaryParsed.diram !== p.salary.value) patch.salary = salaryParsed.diram
  const limits: Record<string, number | null> = {}
  for (const l of p.limits) {
    const d = form.limits[l.name]
    if (d.reset) {
      if (!l.isDefault) limits[l.name] = null
      continue
    }
    const r = parseLimit(d.text)
    if (r.ok && r.diram !== l.value) limits[l.name] = r.diram
  }
  if (Object.keys(limits).length) patch.limits = limits

  const hasErrors = !!nameError || !!salaryError || Object.values(limitErrors).some(Boolean)
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
  function setLimit(name: string, text: string) {
    setForm((f) => f && { ...f, limits: { ...f.limits, [name]: { text, reset: false } } })
  }
  function resetLimit(name: string, def: number) {
    setForm(
      (f) => f && { ...f, limits: { ...f.limits, [name]: { text: def === 0 ? '' : diramToInput(def), reset: true } } },
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
      reloadSession() // effective salary / limits / name everywhere (FR-017)
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

      <div className="card">
        <div className="card-title">Лимиты по категориям</div>
        <div className="hint">Пусто или 0 — без лимита. Действуют для всех месяцев.</div>
        {p.limits.map((l) => {
          const d = form.limits[l.name]
          const r = parseLimit(d.text)
          const changedFromDefault = !d.reset && (!l.isDefault || (r.ok && r.diram !== l.default))
          const err = limitErrors[l.name] ?? (serverField === `limits.${l.name}` ? apiErr!.message : null)
          return (
            <div className="profile-limit" key={l.name}>
              <label htmlFor={'lim-' + l.name}>{l.label}</label>
              <input
                id={'lim-' + l.name}
                className={'input' + (err ? ' invalid' : '')}
                inputMode="decimal"
                autoComplete="off"
                placeholder="без лимита"
                value={d.text}
                onChange={(e) => setLimit(l.name, e.target.value)}
              />
              <div className="meta">
                <span>
                  {d.reset ? 'будет сброшен' : l.isDefault ? 'по умолчанию' : 'свой'} · стандарт{' '}
                  {l.default === 0 ? 'без лимита' : formatSomoni(l.default)}
                </span>
                {changedFromDefault && (
                  <button className="btn-link" onClick={() => resetLimit(l.name, l.default)}>
                    Сбросить
                  </button>
                )}
              </div>
              {err && <div className="error-text" style={{ gridColumn: '1 / -1' }}>{err}</div>}
            </div>
          )
        })}
      </div>

      {error != null && !serverField && <ErrorBanner error={error} onRetry={canSave ? save : undefined} />}
      {serverField && !['displayName', 'salary'].includes(serverField) && !serverField.startsWith('limits.') && (
        <ErrorBanner error={error} />
      )}

      <button className="btn" disabled={!canSave} onClick={save}>
        {saving ? 'Сохраняю…' : 'Сохранить'}
      </button>
    </div>
  )
}
