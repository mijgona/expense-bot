import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useNav } from '../context'
import { useCategories } from '../lib/categories'
import { listHistory, listMonths } from '../lib/api'
import { formatSomoni } from '../lib/money'
import { monthTitle } from '../lib/months'
import type { HistoryGroup, HistoryQuery, MonthAgg, Transaction } from '../lib/types'
import { useAsync } from '../hooks'
import { Loader } from '../components/Loader'
import { ErrorBanner } from '../components/ErrorBanner'
import { TransactionRow } from '../components/TransactionRow'

const GROUPS: { value: HistoryGroup | undefined; label: string }[] = [
  { value: undefined, label: 'Все' },
  { value: 'expense', label: 'Расход' },
  { value: 'income', label: 'Приход' },
  { value: 'savings', label: 'Накопления' },
  { value: 'credit', label: 'Карта' },
]

type Filter = Pick<HistoryQuery, 'group' | 'category' | 'month' | 'q'>

interface PageState {
  items: Transaction[]
  nextCursor: string | null
  total: number
  totalExact: boolean
  loading: boolean
  error: unknown
  loaded: boolean
}

const EMPTY: PageState = { items: [], nextCursor: null, total: 0, totalExact: true, loading: false, error: null, loaded: false }

export function History({ filter: initial }: { filter?: Partial<HistoryQuery> }) {
  const nav = useNav()
  const { all: categories } = useCategories()
  const months = useAsync(() => listMonths(), [])
  const monthAgg = useMemo(() => {
    const m = new Map<string, MonthAgg>()
    for (const a of months.data?.items ?? []) m.set(a.month, a)
    return m
  }, [months.data])

  const [filter, setFilter] = useState<Filter>({
    group: initial?.group,
    category: initial?.category,
    month: initial?.month,
    q: initial?.q,
  })
  const [search, setSearch] = useState(initial?.q ?? '')
  const [page, setPage] = useState<PageState>(EMPTY)
  const seq = useRef(0)

  // Debounce the search box into the filter.
  useEffect(() => {
    const t = setTimeout(() => setFilter((f) => (f.q === (search.trim() || undefined) ? f : { ...f, q: search.trim() || undefined })), 300)
    return () => clearTimeout(t)
  }, [search])

  const load = useCallback(
    async (cursor: string | null, reset: boolean) => {
      const id = reset ? ++seq.current : seq.current
      setPage((p) => ({ ...(reset ? EMPTY : p), loading: true, error: null }))
      try {
        const res = await listHistory({ ...filter, cursor: cursor ?? undefined, limit: 50 })
        if (id !== seq.current) return
        setPage((p) => ({
          items: reset ? res.items : [...p.items, ...res.items],
          nextCursor: res.nextCursor,
          // Exact total (no search) covers the whole filter: take it from the first page.
          // With search the server totals only that page's matches, so accumulate across pages.
          total: reset ? res.total : res.totalExact ? p.total : p.total + res.total,
          totalExact: res.totalExact,
          loading: false,
          error: null,
          loaded: true,
        }))
      } catch (e) {
        if (id !== seq.current) return
        setPage((p) => ({ ...p, loading: false, error: e, loaded: true }))
      }
    },
    [filter],
  )

  useEffect(() => {
    void load(null, true)
  }, [load])

  // Infinite scroll: load the next page when the sentinel becomes visible.
  const sentinel = useRef<HTMLDivElement | null>(null)
  useEffect(() => {
    const el = sentinel.current
    if (!el || !page.nextCursor || page.loading || page.error != null) return
    const io = new IntersectionObserver(
      (entries) => {
        if (entries.some((e) => e.isIntersecting)) void load(page.nextCursor, false)
      },
      { rootMargin: '300px' },
    )
    io.observe(el)
    return () => io.disconnect()
  }, [page.nextCursor, page.loading, page.error, load])

  const filtered = !!(filter.group || filter.category || filter.month || filter.q)
  const showCategory = filter.group === undefined || filter.group === 'expense' || filter.group === 'credit'

  function setGroup(group: HistoryGroup | undefined) {
    setFilter((f) => ({
      ...f,
      group,
      category: group === 'income' || group === 'savings' ? undefined : f.category,
    }))
  }

  function reset() {
    setSearch('')
    setFilter({})
  }

  // Group loaded items by month (they arrive newest first).
  const groups: { month: string; items: Transaction[] }[] = []
  for (const t of page.items) {
    const last = groups[groups.length - 1]
    if (last && last.month === t.month) last.items.push(t)
    else groups.push({ month: t.month, items: [t] })
  }

  return (
    <div className="screen">
      <div className="screen-title">📜 История</div>

      <div className="card filters">
        <div className="chips">
          {GROUPS.map((g) => (
            <button
              key={g.label}
              type="button"
              className={'chip' + (filter.group === g.value ? ' selected' : '')}
              onClick={() => setGroup(g.value)}
            >
              {g.label}
            </button>
          ))}
        </div>
        <div className="row-2">
          {showCategory ? (
            <select
              className="input"
              aria-label="Категория"
              value={filter.category ?? ''}
              onChange={(e) => setFilter((f) => ({ ...f, category: e.target.value || undefined }))}
            >
              <option value="">Все категории</option>
              {categories.map((c) => (
                <option key={c.id} value={c.id}>
                  {c.name}
                  {c.hidden ? ' (скрыта)' : ''}
                </option>
              ))}
            </select>
          ) : (
            <span />
          )}
          <select
            className="input"
            aria-label="Месяц"
            value={filter.month ?? ''}
            onChange={(e) => setFilter((f) => ({ ...f, month: e.target.value || undefined }))}
          >
            <option value="">Все месяцы</option>
            {filter.month && !monthAgg.has(filter.month) && <option value={filter.month}>{monthTitle(filter.month)}</option>}
            {(months.data?.items ?? []).map((m) => (
              <option key={m.month} value={m.month}>
                {monthTitle(m.month)}
              </option>
            ))}
          </select>
        </div>
        <input
          className="input"
          type="search"
          placeholder="🔍 Поиск по описанию"
          maxLength={100}
          value={search}
          onChange={(e) => setSearch(e.target.value)}
        />
        {filtered && (
          <button className="btn-link" onClick={reset}>
            Сбросить фильтры
          </button>
        )}
      </div>

      {page.loaded && page.items.length > 0 && (
        <div className="card row">
          <span>{page.totalExact ? 'Итого' : 'Итого по найденным'}</span>
          <span className="tx-amount">{formatSomoni(page.total)}</span>
        </div>
      )}

      {!page.loaded && page.loading && <Loader />}

      {page.loaded && page.items.length === 0 && page.error == null && !page.loading && (
        <div className="card hint">
          {filtered ? (
            <>
              <div>Ничего не найдено</div>
              <button className="btn-link" onClick={reset}>
                Сбросить фильтры
              </button>
            </>
          ) : (
            <>
              <div>Записей пока нет</div>
              <button className="btn-link" onClick={() => nav.push({ name: 'add', kind: 'expense' })}>
                Добавить
              </button>
            </>
          )}
        </div>
      )}

      {groups.map((g) => {
        const agg = monthAgg.get(g.month)
        return (
          <div className="card" key={g.month}>
            <div className="month-head">
              <span>{monthTitle(g.month)}</span>
              {agg && (
                <span className="hint">
                  💵 {formatSomoni(agg.income)} · 💸 {formatSomoni(agg.expense)}
                </span>
              )}
            </div>
            {g.items.map((t) => (
              <TransactionRow key={t.id} tx={t} onClick={() => nav.push({ name: 'edit', tx: t })} />
            ))}
          </div>
        )
      })}

      {page.error != null && (
        <ErrorBanner error={page.error} onRetry={() => void load(page.loaded && page.items.length ? page.nextCursor : null, !page.items.length)} />
      )}
      {page.loaded && page.loading && <Loader text={filter.q ? 'Ищу дальше…' : 'Загружаю…'} />}
      <div ref={sentinel} className="sentinel" />
    </div>
  )
}
