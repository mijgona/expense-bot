import { tg } from './telegram'
import type {
  AdvisorReport,
  Goal,
  GoalsResponse,
  NewGoal,
  NewTransaction,
  Session,
  Summary,
  Transaction,
  WriteResult,
} from './types'

const BASE: string = import.meta.env.VITE_API_URL ?? ''

const DEFAULT_TIMEOUT_MS = 15_000
const ADVISOR_TIMEOUT_MS = 90_000

export class ApiError extends Error {
  status: number
  code: string
  field?: string

  constructor(status: number, code: string, message: string, field?: string) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.code = code
    this.field = field
  }
}

async function request<T>(method: 'GET' | 'POST', path: string, body?: unknown): Promise<T> {
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

  let data: unknown = null
  const text = await res.text()
  if (text) {
    try {
      data = JSON.parse(text)
    } catch {
      data = null
    }
  }

  if (!res.ok) {
    const err = (data as { error?: { code?: string; message?: string; field?: string } } | null)?.error
    throw new ApiError(
      res.status,
      err?.code ?? (res.status >= 500 ? 'internal' : 'unknown'),
      err?.message ?? `Ошибка сервера (${res.status})`,
      err?.field,
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

export const listGoals = () => request<GoalsResponse>('GET', '/api/goals')

export const addGoal = (body: NewGoal) => request<Goal>('POST', '/api/goals', body)

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
