import { useCategories } from '../lib/categories'

/** Picker of visible categories (values are IDs). A selected hidden category is shown as a chip but not offered. */
export function CategoryGrid({ selected, onSelect }: { selected: string | null; onSelect(id: string): void }) {
  const { visible, byId, labelOf } = useCategories()
  const sel = byId(selected)
  const selectedHidden = selected != null && (!sel || sel.hidden)
  return (
    <div className="field">
      <label>Категория</label>
      {selectedHidden && (
        <div className="hint">
          Выбрано: {labelOf(selected)} <span className="tag">скрыта</span>
        </div>
      )}
      <div className="cat-grid">
        {visible.map((c) => (
          <button
            key={c.id}
            type="button"
            className={'cat' + (selected === c.id ? ' selected' : '')}
            onClick={() => onSelect(c.id)}
          >
            {c.name}
          </button>
        ))}
      </div>
    </div>
  )
}
