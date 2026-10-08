// Mirrors specs/004-telegram-mini-app/contracts/api.openapi.yaml. Amounts are integer diram.

export type Kind =
  | 'expense'
  | 'income'
  | 'savings_deposit'
  | 'savings_withdrawal'
  | 'credit_purchase'
  | 'credit_repayment'

export interface Category {
  name: string
  label: string
  /** Effective per-user limit in diram; 0 = no limit. */
  limit: number
}

export interface Session {
  user: {
    id: number
    firstName: string
    /** Effective display name (profile override or firstName). */
    displayName: string
    username: string
    registeredAt: string
    isNew: boolean
  }
  salary: number
  categories: Category[]
  currentMonth: string
  firstMonth: string | null
}

export type LimitStatus = 'ok' | 'warn' | 'over' | 'none'

export interface CategoryLine {
  name: string
  label: string
  spent: number
  limit: number | null
  status: LimitStatus
}

export interface Summary {
  month: string
  isCurrent: boolean
  income: number
  expense: number
  savingsNet: number
  creditCharged: number
  creditRepaid: number
  carryOver: number
  remaining: number
  savingsBalance: number
  creditDebt: number
  daysLeft?: number | null
  dailyBudget?: number | null
  categories: CategoryLine[]
}

export interface NewTransaction {
  clientId: string
  kind: Kind
  amount: number
  category?: string
  note?: string
  /** YYYY-MM-DD (Dushanbe), not after today; default today. */
  date?: string
}

export interface Transaction {
  id: string
  kind: Kind
  category?: string | null
  amount: number
  note: string
  month: string
  /** Business date/time; month derives from it. */
  occurredAt: string
  /** When it was entered. */
  createdAt: string
  editedAt: string | null
  version: number
}

export interface TransactionPatch {
  version: number
  requestId: string
  amount?: number
  category?: string
  note?: string
  date?: string
}

export type HistoryGroup = 'expense' | 'income' | 'savings' | 'credit'

export interface HistoryQuery {
  group?: HistoryGroup
  category?: string
  month?: string
  q?: string
  cursor?: string
  limit?: number
}

export interface HistoryPage {
  items: Transaction[]
  nextCursor: string | null
  total: number
  totalExact: boolean
}

export interface MonthAgg {
  month: string
  income: number
  expense: number
  savingsNet: number
  creditCharged: number
  creditRepaid: number
  cashNet: number
}

export interface ProfileValue<T> {
  value: T
  default: T
  isDefault: boolean
}

export interface ProfileLimit extends ProfileValue<number> {
  name: string
  label: string
}

export interface Profile {
  telegram: { firstName: string; username: string }
  displayName: ProfileValue<string>
  salary: ProfileValue<number>
  limits: ProfileLimit[]
  updatedAt?: string | null
}

export interface ProfilePatch {
  /** null resets to the default */
  displayName?: string | null
  salary?: number | null
  limits?: Record<string, number | null>
}

export interface WriteResult {
  transaction: Transaction
  summary: Summary
}

export interface Goal {
  id: string
  name: string
  target: number
  quarter: string
  status: 'active' | 'done'
  note: string
  progress: number
  version: number
}

export interface GoalPatch {
  version: number
  requestId: string
  name?: string
  target?: number
  quarter?: string
  note?: string
  status?: 'active' | 'done'
}

export interface GoalsResponse {
  savingsBalance: number
  items: Goal[]
}

export interface NewGoal {
  clientId: string
  name: string
  target: number
  quarter: string
  note?: string
}

export interface AdvisorReport {
  markdown: string
  generatedAt: string
}
