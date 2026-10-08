import { useEffect, useRef, useState } from 'react'
import { useSessionReload } from '../context'
import { addCategory, ApiError, listCategories, newClientId, orderCategories, patchCategory } from '../lib/api'
import { diramToInput, formatSomoni, parseAmountInput } from '../lib/money'
import { haptic, toast } from '../lib/telegram'
import type { Category, CategoryList } from '../lib/types'
import { useAsync } from '../hooks'
import { AmountInput } from '../components/AmountInput'
import { ErrorBanner } from '../components/ErrorBanner'
import { Loader } from '../components/Loader'

const MAX_NAME = 30

/** Limit input: empty or 0 means "no limit" (0). */
function parseLimit(text: string): { ok: true; diram: number } | { ok: false; error: string } {
  const t = text.trim()
  if (t === '' || /^0+([.,]0*)?$/.test(t)) return { ok: true, diram: 0 }
  return parseAmountInput(t)
}

function nameError(name: string): string | null {
  const n = name.trim()
  if (!n) return 'Введите название'
  if ([...n].length > MAX_NAME) return `Не длиннее ${MAX_NAME} символов`
  return null
}

/** Display order: visible by position, then hidden by position. */
function displayOrder(items: Category[]): Category[] {
  const byPos = [...items].sort((a, b) => a.position - b.position)
  return [...byPos.filter((c) => !c.hidden), ...byPos.filter((c) => c.hidden)]
}

interface FieldErrors {
  name?: string
  limit?: string
}

/** Category management — reachable only from Profile (feature 006). */
export function Categories() {
  const reloadSession = useSessionReload()
  const initial = useAsync(() => listCategories(), [])
  const [list, setList] = useState<CategoryList | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<unknown>(null)
  const [conflict, setConflict] = useState(false)

  // Add form
  const [adding, setAdding] = useState(false)
  const [newName, setNewName] = useState('')
  const [newLimit, setNewLimit] = useState('')
  const [addErrors, setAddErrors] = useState<FieldErrors>({})
  const clientId = useRef<string | null>(null) // kept until the add succeeds (idempotent retry)

  // Inline edit
  const [editId, setEditId] = useState<string | null>(null)
  const [editName, setEditName] = useState('')
  const [editLimit, setEditLimit] = useState('')
  const [editErrors, setEditErrors] = useState<FieldErrors>({})

  useEffect(() => {
    if (initial.data) setList(initial.data)
  }, [initial.data])

  if (!list) {
    return (
      <div className="screen">
        <div className="screen-title">🏷 Категории</div>
        {initial.error != null ? <ErrorBanner error={initial.error} onRetry={initial.reload} /> : <Loader />}
      </div>
    )
  }

  const items = displayOrder(list.items)
  const visibleCount = list.items.filter((c) => !c.hidden).length

  function applied(next: CategoryList) {
    setList(next)
    setConflict(false)
    setError(null)
    reloadSession() // pickers, labels and limits update everywhere
    haptic('success')
  }

  /** Shared error handling; returns field errors for inline display. */
  function handleError(e: unknown): FieldErrors {
    haptic('error')
    if (e instanceof ApiError) {
      if (e.code === 'conflict') {
        const cur = e.current as CategoryList | undefined
        if (cur && Array.isArray(cur.items)) setList(cur)
        else initial.reload()
        setConflict(true)
        return {}
      }
      if (e.code === 'duplicate') return { name: e.message || 'Такая категория уже есть' }
      if (e.code === 'limit_reached' || e.code === 'last_visible') {
        toast(e.message)
        return {}
      }
      if (e.code === 'validation' && (e.field === 'name' || e.field === 'limit')) {
        return { [e.field]: e.message }
      }
    }
    setError(e)
    return {}
  }

  async function submitAdd() {
    const nErr = nameError(newName)
    const lim = parseLimit(newLimit)
    const errs: FieldErrors = {}
    if (nErr) errs.name = nErr
    if (!lim.ok) errs.limit = lim.error
    setAddErrors(errs)
    if (nErr || !lim.ok) return
    if (!clientId.current) clientId.current = newClientId()
    setBusy(true)
    try {
      const next = await addCategory({ clientId: clientId.current, name: newName.trim(), limit: lim.diram })
      clientId.current = null
      setNewName('')
      setNewLimit('')
      setAdding(false)
      setAddErrors({})
      applied(next)
      toast('Категория добавлена')
    } catch (e) {
      const errs = handleError(e)
      // A rejected add (duplicate / validation / cap) needs a fresh clientId for the next attempt.
      if (e instanceof ApiError && e.status !== 0 && e.status < 500) clientId.current = null
      setAddErrors(errs)
    } finally {
      setBusy(false)
    }
  }

  function startEdit(c: Category) {
    setEditId(c.id)
    setEditName(c.name)
    setEditLimit(c.limit === 0 ? '' : diramToInput(c.limit))
    setEditErrors({})
    setAdding(false)
  }

  async function submitEdit(c: Category) {
    const nErr = nameError(editName)
    const lim = parseLimit(editLimit)
    const errs: FieldErrors = {}
    if (nErr) errs.name = nErr
    if (!lim.ok) errs.limit = lim.error
    setEditErrors(errs)
    if (nErr || !lim.ok || !list) return
    const body: { version: number; name?: string; limit?: number } = { version: list.version }
    if (editName.trim() !== c.name) body.name = editName.trim()
    if (lim.diram !== c.limit) body.limit = lim.diram
    if (body.name === undefined && body.limit === undefined) {
      setEditId(null)
      return
    }
    setBusy(true)
    try {
      applied(await patchCategory(c.id, body))
      setEditId(null)
      toast('Сохранено')
    } catch (e) {
      setEditErrors(handleError(e))
    } finally {
      setBusy(false)
    }
  }

  async function toggleHidden(c: Category) {
    if (!list) return
    setBusy(true)
    try {
      applied(await patchCategory(c.id, { version: list.version, hidden: !c.hidden }))
      toast(c.hidden ? 'Категория снова доступна' : 'Категория скрыта')
    } catch (e) {
      handleError(e)
    } finally {
      setBusy(false)
    }
  }

  async function move(index: number, dir: -1 | 1) {
    if (!list) return
    const j = index + dir
    if (j < 0 || j >= items.length) return
    const ids = items.map((c) => c.id)
    ;[ids[index], ids[j]] = [ids[j], ids[index]]
    setBusy(true)
    try {
      applied(await orderCategories({ version: list.version, ids }))
    } catch (e) {
      handleError(e)
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="screen">
      <div className="screen-title">🏷 Категории</div>

      {conflict && <div className="banner">Список изменили на другом устройстве. Показана актуальная версия.</div>}
      {error != null && <ErrorBanner error={error} />}

      <div className="card">
        {items.map((c, i) => (
          <div className={'cat-row' + (c.hidden ? ' hidden' : '')} key={c.id}>
            {editId === c.id ? (
              <div className="cat-edit">
                <div className="field">
                  <label htmlFor={'cname-' + c.id}>Название</label>
                  <input
                    id={'cname-' + c.id}
                    className={'input' + (editErrors.name ? ' invalid' : '')}
                    maxLength={MAX_NAME}
                    autoFocus
                    value={editName}
                    onChange={(e) => setEditName(e.target.value)}
                  />
                  {editErrors.name && <div className="error-text">{editErrors.name}</div>}
                </div>
                <div className="field">
                  <label htmlFor={'climit-' + c.id}>Лимит в месяц, с.</label>
                  <input
                    id={'climit-' + c.id}
                    className={'input' + (editErrors.limit ? ' invalid' : '')}
                    inputMode="decimal"
                    autoComplete="off"
                    placeholder="без лимита"
                    value={editLimit}
                    onChange={(e) => setEditLimit(e.target.value)}
                  />
                  {editErrors.limit && <div className="error-text">{editErrors.limit}</div>}
                </div>
                <div className="goal-actions">
                  <button className="btn" disabled={busy} onClick={() => submitEdit(c)}>
                    {busy ? 'Сохраняю…' : 'Сохранить'}
                  </button>
                  <button className="btn-link" onClick={() => setEditId(null)}>
                    Отмена
                  </button>
                </div>
              </div>
            ) : (
              <>
                <div className="cat-main" onClick={() => startEdit(c)}>
                  <div>
                    {c.name} {c.hidden && <span className="tag">скрыта</span>}
                  </div>
                  <div className="hint">{c.limit === 0 ? 'без лимита' : `лимит ${formatSomoni(c.limit)}`}</div>
                </div>
                <div className="cat-tools">
                  <button
                    className="btn-link"
                    aria-label="Выше"
                    disabled={busy || i === 0}
                    onClick={() => move(i, -1)}
                  >
                    ▲
                  </button>
                  <button
                    className="btn-link"
                    aria-label="Ниже"
                    disabled={busy || i === items.length - 1}
                    onClick={() => move(i, 1)}
                  >
                    ▼
                  </button>
                  <button
                    className="btn-link"
                    disabled={busy || (!c.hidden && visibleCount <= 1)}
                    onClick={() => toggleHidden(c)}
                  >
                    {c.hidden ? 'Показать' : 'Скрыть'}
                  </button>
                </div>
              </>
            )}
          </div>
        ))}
      </div>

      {adding ? (
        <div className="card">
          <div className="card-title">Новая категория</div>
          <div className="field">
            <label htmlFor="newcat">Название</label>
            <input
              id="newcat"
              className={'input' + (addErrors.name ? ' invalid' : '')}
              maxLength={MAX_NAME}
              placeholder="🐱 Кошка"
              autoFocus
              value={newName}
              onChange={(e) => {
                setNewName(e.target.value)
                clientId.current = null
              }}
            />
            {addErrors.name && <div className="error-text">{addErrors.name}</div>}
          </div>
          <AmountInput
            label="Лимит в месяц, с. (пусто — без лимита)"
            autoFocus={false}
            value={newLimit}
            onChange={(v) => {
              setNewLimit(v)
              clientId.current = null
            }}
            error={addErrors.limit ?? null}
          />
          <div className="goal-actions">
            <button className="btn" disabled={busy} onClick={submitAdd}>
              {busy ? 'Добавляю…' : 'Добавить'}
            </button>
            <button className="btn-link" onClick={() => setAdding(false)}>
              Отмена
            </button>
          </div>
        </div>
      ) : (
        <button
          className="btn btn-secondary"
          onClick={() => {
            setAdding(true)
            setEditId(null)
          }}
        >
          ➕ Новая категория
        </button>
      )}

      <div className="hint">
        Скрытые категории не предлагаются для новых записей, но старые записи с ними остаются в истории и отчётах.
      </div>
    </div>
  )
}
