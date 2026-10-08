import { useSession } from '../context'

export function CategoryGrid({ selected, onSelect }: { selected: string | null; onSelect(name: string): void }) {
  const { categories } = useSession()
  return (
    <div className="field">
      <label>Категория</label>
      <div className="cat-grid">
        {categories.map((c) => (
          <button
            key={c.name}
            type="button"
            className={'cat' + (selected === c.name ? ' selected' : '')}
            onClick={() => onSelect(c.name)}
          >
            {c.label}
          </button>
        ))}
      </div>
    </div>
  )
}
