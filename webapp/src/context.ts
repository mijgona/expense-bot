import { createContext, useContext } from 'react'
import type { HistoryQuery, Session, Transaction } from './lib/types'

export type AddKind = 'expense' | 'income' | 'credit_purchase'

export type Screen =
  | { name: 'home' }
  | { name: 'add'; kind: AddKind }
  | { name: 'report'; month: string }
  /** mode 'deposit' preselects the deposit form (home «Отложить» / «Пополнить», 008). */
  | { name: 'savings'; mode?: 'deposit' }
  | { name: 'credit' }
  | { name: 'goals' }
  | { name: 'advisor' }
  | { name: 'history'; filter?: Partial<HistoryQuery> }
  | { name: 'edit'; tx: Transaction }
  | { name: 'profile' }
  /** Category management — opened only from Profile (feature 006). */
  | { name: 'categories' }

/** Bottom tab bar sections (feature 008). */
export type Tab = 'home' | 'history' | 'report' | 'profile'

export interface Nav {
  /** Opens a screen on top of the current tab (the tab bar hides). */
  push(screen: Screen): void
  /** Closes the top screen; at the last one, returns to the tab root. */
  pop(): void
  /** Current tab. */
  tab: Tab
  /** Switches tab and clears the stack; ignored for the current tab. */
  setTab(tab: Tab): void
}

export const NavContext = createContext<Nav | null>(null)
export const SessionContext = createContext<Session | null>(null)

export function useNav(): Nav {
  const nav = useContext(NavContext)
  if (!nav) throw new Error('NavContext missing')
  return nav
}

export function useSession(): Session {
  const s = useContext(SessionContext)
  if (!s) throw new Error('SessionContext missing')
  return s
}

/** Refetches /api/session (effective salary, display name, categories) after profile / category changes. */
export const SessionReloadContext = createContext<(() => void) | null>(null)

export function useSessionReload(): () => void {
  const r = useContext(SessionReloadContext)
  if (!r) throw new Error('SessionReloadContext missing')
  return r
}
