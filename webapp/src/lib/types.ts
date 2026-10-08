// Mirrors specs/006-custom-categories/contracts/api.openapi.yaml (v2.0.0). Amounts are integer diram.
// Categories are per-user and referenced by ID (c_…) everywhere.

export type Kind =
  | 'expense'
  | 'income'
  | 'savings_deposit'
  | 'savings_withdrawal'
  | 'credit_purchase'
  | 'credit_repayment'

export interface Category {
  /** Permanent ID (c_…); records, filters and summary lines reference it. */
  id: string
  /** Current display name (emoji allowed, 1–30 chars). */
  name: string
  /** Monthly limit in diram; 0 = no limit. */
  limit: number
  /** Hidden categories are not offered for new records / category changes. */
  hidden: boolean
  position: number
  isDefault: boolean
}

export interface CategoryList {
  /** categoriesVersion — send back on PATCH / order. */
  version: number
  /** All categories incl. hidden, by position. */
  items: Category[]
}

export interface NewCategory {
  clientId: string
  name: string
  limit?: number
}

export interface CategoryPatch {
  version: number
  name?: string
  limit?: number
  hidden?: boolean
}

export interface CategoryOrder {
  version: number
  ids: string[]
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
  /** All of the user's categories incl. hidden, ordered by position. Pickers show only hidden=false. */
  categories: Category[]
  categoriesVersion: number
  currentMonth: string
  firstMonth: string | null
}

export type LimitStatus = 'ok' | 'warn' | 'over' | 'none'

export interface CategoryLine {
  /** Category ID. */
  id: string
  /** Current category name. */
  label: string
  spent: number
  limit: number | null
  status: LimitStatus
  /** Hidden categories appear only when spent > 0 in the month. */
  hidden: boolean
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
  /** Split mode, current month only; daysLeft / dailyBudget then refer to it. */
  nextPayday?: NextPayday | null
  /** Spending pace for the month / pay period; current month only, null when budget ≤ 0 (008). */
  pace?: Pace | null
  /** Current quarter progress; current month only (008). */
  quarter?: QuarterInfo | null
  categories: CategoryLine[]
}

export interface Pace {
  /** Expenses (kind=expense) in the period, diram. */
  spent: number
  /** Money available in the period, diram. */
  budget: number
  /** Share of the period that has passed, today included (0..1). */
  elapsed: number
  period: 'month' | 'advance' | 'rest'
}

export interface QuarterInfo {
  /** "Q4 2026" — same format as Goal.quarter. */
  name: string
  /** Share of the quarter that has passed (0..1). */
  elapsed: number
  /** Days left in the quarter, today included. */
  daysLeft: number
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

export type SalaryMode = 'single' | 'split'

export interface Profile {
  telegram: { firstName: string; username: string }
  displayName: ProfileValue<string>
  salary: ProfileValue<number>
  /** 'single' = whole salary on the last day; 'split' = advance on the 15th + rest on the last day. */
  salaryMode: SalaryMode
  /** Effective advance (split mode); default = half the salary in whole somoni. */
  advance: ProfileValue<number>
  /** salary − advance (split) or salary (single); read-only. */
  rest: number
  /** Bot payday reminders. */
  salaryReminders: boolean
  updatedAt?: string | null
}

export interface ProfilePatch {
  /** null resets to the default */
  displayName?: string | null
  salary?: number | null
  salaryMode?: SalaryMode
  /** 100 … salary − 100 diram; null = half */
  advance?: number | null
  salaryReminders?: boolean
}

export type PayoutKind = 'advance' | 'rest' | 'full'

export type PayoutStatus = 'upcoming' | 'due' | 'recorded' | 'dismissed'

export interface Payout {
  month: string
  kind: PayoutKind
  /** Always null: the amount is what the user records as income. */
  amount: number | null
  /** YYYY-MM-DD */
  payday: string
  note: string
  status: PayoutStatus
  /** p_<month>_<kind> when recorded */
  transactionId: string | null
}

export interface RecordPayout {
  amount: number
  /** YYYY-MM-DD; default the payday, not after today */
  date?: string
}

export interface NextPayday {
  /** YYYY-MM-DD */
  date: string
  kind: PayoutKind
  daysLeft: number
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
