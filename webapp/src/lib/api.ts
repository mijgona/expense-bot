import { tg } from './telegram'
import type {
  AdvisorReport,
  CategoryList,
  CategoryOrder,
  CategoryPatch,
  NewCategory,
  Goal,
  GoalPatch,
  GoalsResponse,
  HistoryPage,
  HistoryQuery,
  MonthAgg,
  NewGoal,
  NewTransaction,
  Payout,
  PayoutKind,
  Profile,
  ProfilePatch,
  RecordPayout,
  Session,
  Summary,
  Transaction,
  TransactionPatch,
  WriteResult,
} from './types'

const BASE: string = import.meta.env.VITE_API_URL ?? ''

const DEFAULT_TIMEOUT_MS = 15_000
const ADVISOR_TIMEOUT_MS = 90_000

interface ErrorBody {
  code?: string
  message?: string
  field?: string
  /** Balance rule: first date where the balance would break. */
  at?: string
  /** Balance rule: would-be balance at `at` (diram, negative). */
  balance?: number
  /** Conflict: the current record, goal or category list. */
  current?: unknown
}

export class ApiError extends Error {
  status: number
  code: string
  field?: string
  at?: string
  balance?: number
  current?: unknown

  constructor(status: number, code: string, message: string, field?: string, extra?: ErrorBody) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.code = code
    this.field = field
    this.at = extra?.at
    this.balance = extra?.balance
    this.current = extra?.current
  }
}

type Method = 'GET' | 'POST' | 'PATCH' | 'DELETE'

/** Returns null for 204 No Content. */
async function request<T>(method: Method, path: string, body?: unknown): Promise<T> {
  const headers: Record<string, string> = {}
  const initData = tg()?.initData
  if (initData) headers.Authorization = 'tma ' + initData
  if (body !== undefined) headers['Content-Type'] = 'application/json'

  const ctrl = new AbortController()
  const timeout = path.startsWith('/api/advisor/') ? ADVISOR_TIMEOUT_MS : DEFAULT_TIMEOUT_MS
  const timer = setTimeout(() => ctrl.abort(), timeout)

  let res: Response
  try {
    res = await fetch(BASE + path, {
      method,
      headers,
      body: body === undefined ? undefined : JSON.stringify(body),
      signal: ctrl.signal,
    })
  } catch {
    throw new ApiError(0, 'network', 'Нет связи с сервером. Проверьте интернет и попробуйте ещё раз.')
  } finally {
    clearTimeout(timer)
  }

  if (res.status === 204) return null as T

  let data: unknown = null
  const text = await res.text()
  if (text) {
    try {
      data = JSON.parse(text)
    } catch {
      data = null
    }
  }

  // A 200 with non-JSON body means we hit the static site, not the API (VITE_API_URL missing/wrong).
  if (res.ok && data === null) {
    throw new ApiError(
      res.status,
      'misconfigured',
      'Приложение не может связаться с сервером: не настроен адрес API (VITE_API_URL).',
    )
  }

  if (!res.ok) {
    const err = (data as { error?: ErrorBody } | null)?.error
    throw new ApiError(
      res.status,
      err?.code ?? (res.status >= 500 ? 'internal' : 'unknown'),
      err?.message ?? `Ошибка сервера (${res.status})`,
      err?.field,
      err,
    )
  }
  return data as T
}

export const getSession = () => request<Session>('POST', '/api/session')

export const getSummary = (month?: string) =>
  request<Summary>('GET', '/api/summary' + (month ? `?month=${encodeURIComponent(month)}` : ''))

export const listTransactions = (month: string, limit = 100) =>
  request<{ items: Transaction[] }>(
    'GET',
    `/api/transactions?month=${encodeURIComponent(month)}&limit=${limit}`,
  )

export const addTransaction = (body: NewTransaction) =>
  request<WriteResult>('POST', '/api/transactions', body)

function qs(params: Record<string, string | number | undefined | null>): string {
  const p = new URLSearchParams()
  for (const [k, v] of Object.entries(params)) {
    if (v !== undefined && v !== null && v !== '') p.set(k, String(v))
  }
  const s = p.toString()
  return s ? '?' + s : ''
}

/** History: all records, newest first, filterable, cursor-paged. */
export const listHistory = (q: HistoryQuery) =>
  request<HistoryPage>(
    'GET',
    '/api/transactions' +
      qs({ group: q.group, category: q.category, month: q.month, q: q.q, cursor: q.cursor, limit: q.limit }),
  )

export const patchTransaction = (id: string, body: TransactionPatch) =>
  request<WriteResult>('PATCH', `/api/transactions/${encodeURIComponent(id)}`, body)

/** Deletes a record. Resolves with the new summary, or null when it was already gone (204). */
export const deleteTransaction = (id: string, version: number) =>
  request<{ summary: Summary } | null>(
    'DELETE',
    `/api/transactions/${encodeURIComponent(id)}` + qs({ version }),
  )

export const listMonths = (from?: string, to?: string) =>
  request<{ items: MonthAgg[] }>('GET', '/api/months' + qs({ from, to }))

export const getProfile = () => request<Profile>('GET', '/api/profile')

export const patchProfile = (body: ProfilePatch) => request<Profile>('PATCH', '/api/profile', body)

export const listCategories = () => request<CategoryList>('GET', '/api/categories')

export const addCategory = (body: NewCategory) => request<CategoryList>('POST', '/api/categories', body)

export const patchCategory = (id: string, body: CategoryPatch) =>
  request<CategoryList>('PATCH', `/api/categories/${encodeURIComponent(id)}`, body)

export const orderCategories = (body: CategoryOrder) =>
  request<CategoryList>('POST', '/api/categories/order', body)

/** Expected salary payments with status (current month by default; may include last month's last-day payment). */
export const listPayouts = (month?: string) =>
  request<{ items: Payout[] }>('GET', '/api/payouts' + qs({ month }))

/** Records a payment as income. Idempotent: a replay returns the existing record (200). */
export const recordPayout = (month: string, kind: PayoutKind, body: RecordPayout) =>
  request<WriteResult>(
    'POST',
    `/api/payouts/${encodeURIComponent(month)}/${encodeURIComponent(kind)}/record`,
    body,
  )

/** Hides the offer for this payment this month without recording anything. */
export const dismissPayout = (month: string, kind: PayoutKind) =>
  request<Payout>('POST', `/api/payouts/${encodeURIComponent(month)}/${encodeURIComponent(kind)}/dismiss`)

export const listGoals = () => request<GoalsResponse>('GET', '/api/goals')

export const addGoal = (body: NewGoal) => request<Goal>('POST', '/api/goals', body)

export const patchGoal = (id: string, body: GoalPatch) =>
  request<Goal>('PATCH', `/api/goals/${encodeURIComponent(id)}`, body)

/** Deletes a goal; 204 both when deleted and when already gone. */
export const deleteGoal = (id: string, version: number) =>
  request<null>('DELETE', `/api/goals/${encodeURIComponent(id)}` + qs({ version }))

export const advisorReport = () => request<AdvisorReport>('POST', '/api/advisor/report')

/** crypto.randomUUID with a fallback for older WebViews. */
export function newClientId(): string {
  const c: Crypto = globalThis.crypto
  if (typeof c.randomUUID === 'function') return c.randomUUID()
  const b = c.getRandomValues(new Uint8Array(16))
  b[6] = (b[6] & 0x0f) | 0x40
  b[8] = (b[8] & 0x3f) | 0x80
  const h = Array.from(b, (x) => x.toString(16).padStart(2, '0')).join('')
  return `${h.slice(0, 8)}-${h.slice(8, 12)}-${h.slice(12, 16)}-${h.slice(16, 20)}-${h.slice(20)}`
}
