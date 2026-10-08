import { createContext, useContext } from 'react'
import type { Session } from './lib/types'

export type AddKind = 'expense' | 'income' | 'credit_purchase'

export type Screen =
  | { name: 'home' }
  | { name: 'add'; kind: AddKind }
  | { name: 'report'; month: string }
  | { name: 'savings' }
  | { name: 'credit' }
  | { name: 'goals' }
  | { name: 'advisor' }

export interface Nav {
  push(screen: Screen): void
  pop(): void
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
