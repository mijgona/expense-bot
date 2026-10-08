import { useMemo } from 'react'
import { useSession } from '../context'
import type { Category } from './types'

export interface Categories {
  /** All categories incl. hidden, by position. */
  all: Category[]
  /** Visible categories by position — the only ones offered in pickers. */
  visible: Category[]
  byId(id: string | null | undefined): Category | undefined
  /** Current name; unknown IDs (transient after a conversion) fall back to the raw value. */
  labelOf(id: string | null | undefined): string
}

export function useCategories(): Categories {
  const { categories } = useSession()
  return useMemo(() => {
    const all = [...categories].sort((a, b) => a.position - b.position)
    const map = new Map(all.map((c) => [c.id, c]))
    return {
      all,
      visible: all.filter((c) => !c.hidden),
      byId: (id) => (id ? map.get(id) : undefined),
      labelOf: (id) => (id ? (map.get(id)?.name ?? id) : ''),
    }
  }, [categories])
}
