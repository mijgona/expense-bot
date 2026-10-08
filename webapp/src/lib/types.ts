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
  limit: number
}

export interface Session {
  user: {
    id: number
    firstName: string
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
}

export interface Transaction {
  id: string
  kind: Kind
  category?: string | null
  amount: number
  note: string
  month: string
  createdAt: string
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
