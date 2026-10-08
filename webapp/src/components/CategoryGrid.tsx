import { useRef, useState } from 'react'
import { useSessionReload } from '../context'
import { addCategory, ApiError, newClientId } from '../lib/api'
import { useCategories } from '../lib/categories'
import { haptic } from '../lib/telegram'
import type { Category } from '../lib/types'

const MAX_NAME = 30

/**
 * Picker of visible categories (values are IDs). A selected hidden category is shown as a chip but not offered.
 * «+ Новая» adds a category in place (no limit; limits are set on the «Категории» screen) and selects it.
 */
export function CategoryGrid({ selected, onSelect }: { selected: string | null; onSelect(id: string): void }) {
  const { visible, byId, labelOf } = useCategories()
  const reloadSession = useSessionReload()
  // The list returned by POST /api/categories, shown until the session reload brings it in.
  const [fresh, setFresh] = useState<Category[] | null>(null)
  const [adding, setAdding] = useState(false)
  const [name, setName] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const clientId = useRef<string | null>(null)

  const freshVisible = fresh && [...fresh].filter((c) => !c.hidden).sort((a, b) => a.position - b.position)
  const items = freshVisible && freshVisible.length > visible.length ? freshVisible : visible
  const sel = byId(selected) ?? fresh?.find((c) => c.id === selected)
  const selectedHidden = selected != null && (!sel || sel.hidden)

  async function submit() {
    const n = name.trim()
    if (!n) return setError('Введите название')
    if ([...n].length > MAX_NAME) return setError(`Не длиннее ${MAX_NAME} символов`)
    if (!clientId.current) clientId.current = newClientId()
    setBusy(true)
    setError(null)
    try {
      const list = await addCategory({ clientId: clientId.current, name: n, limit: 0 })
      const known = new Set(visible.map((c) => c.id))
      const created = list.items.find((c) => !known.has(c.id) && c.name === n) ?? list.items.find((c) => c.name === n)
      clientId.current = null
      setFresh(list.items)
      setName('')
      setAdding(false)
      reloadSession()
      haptic('success')
      if (created) onSelect(created.id)
    } catch (e) {
      // A rejected add (duplicate / validation / cap) needs a fresh clientId for the next attempt.
      if (e instanceof ApiError && e.status !== 0 && e.status < 500) clientId.current = null
      setError(e instanceof ApiError ? e.message : 'Не удалось добавить категорию')
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="field">
      <label>Категория</label>
      {selectedHidden && (
        <div className="hint">
          Выбрано: {labelOf(selected)} <span className="tag">скрыта</span>
        </div>
      )}
      <div className="cat-grid">
        {items.map((c) => (
          <button
            key={c.id}
            type="button"
            className={'cat' + (selected === c.id ? ' selected' : '')}
            onClick={() => onSelect(c.id)}
          >
            {c.name}
          </button>
        ))}
        {!adding && (
          <button type="button" className="cat cat-add" onClick={() => setAdding(true)}>
            + Новая
          </button>
        )}
      </div>
      {adding && (
        <div className="cat-new">
          <input
            className={'input' + (error ? ' invalid' : '')}
            placeholder="Название категории"
            maxLength={MAX_NAME * 2}
            autoFocus
            value={name}
            onChange={(e) => setName(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter') {
                e.preventDefault()
                void submit()
              }
            }}
          />
          {error && <div className="error-text">{error}</div>}
          <div className="goal-actions">
            <button type="button" className="btn" disabled={busy} onClick={() => void submit()}>
              {busy ? 'Добавляю…' : 'Добавить'}
            </button>
            <button
              type="button"
              className="btn-link"
              disabled={busy}
              onClick={() => {
                setAdding(false)
                setName('')
                setError(null)
              }}
            >
              Отмена
            </button>
          </div>
        </div>
      )}
    </div>
  )
}
