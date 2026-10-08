import { lazy, Suspense, useCallback, useMemo, useState } from 'react'
import { NavContext, SessionContext, type Nav, type Screen } from './context'
import { ApiError, getSession } from './lib/api'
import { isInsideTelegram, useBackButton } from './lib/telegram'
import { useAsync } from './hooks'
import { Loader } from './components/Loader'
import { ErrorBanner } from './components/ErrorBanner'
import { OutsideTelegram } from './screens/OutsideTelegram'
import { Denied, Reopen } from './screens/Denied'
import { Home } from './screens/Home'
import { AddTransaction } from './screens/AddTransaction'
import { Report } from './screens/Report'
import { Savings } from './screens/Savings'
import { Credit } from './screens/Credit'
import { Goals } from './screens/Goals'

// Advisor pulls in react-markdown (~45 KB gz); load it only when the screen is opened.
const Advisor = lazy(() => import('./screens/Advisor').then((m) => ({ default: m.Advisor })))

// In dev (vite) the Go server authenticates via DEV_USER_ID, so a plain browser is allowed.
const allowOutside = import.meta.env.DEV

export function App() {
  if (!allowOutside && !isInsideTelegram()) return <OutsideTelegram />
  return <Bootstrap />
}

function Bootstrap() {
  // POST /api/session registers the user on every launch (FR-002).
  const session = useAsync(() => getSession(), [])

  if (session.error instanceof ApiError) {
    if (session.error.status === 403) return <Denied />
    if (session.error.status === 401) return <Reopen />
  }
  if (session.error != null) {
    return (
      <div className="screen">
        <ErrorBanner error={session.error} onRetry={session.reload} />
      </div>
    )
  }
  if (!session.data) return <Loader text="Загрузка…" />

  return (
    <SessionContext.Provider value={session.data}>
      <Navigator />
    </SessionContext.Provider>
  )
}

function Navigator() {
  const [stack, setStack] = useState<Screen[]>([{ name: 'home' }])
  const push = useCallback((s: Screen) => setStack((st) => [...st, s]), [])
  const pop = useCallback(() => setStack((st) => (st.length > 1 ? st.slice(0, -1) : st)), [])
  const nav = useMemo<Nav>(() => ({ push, pop }), [push, pop])

  useBackButton(stack.length > 1 ? pop : null)

  const top = stack[stack.length - 1]
  // Only the top screen is mounted, so a screen refetches its data whenever it becomes visible again.
  return (
    <NavContext.Provider value={nav}>
      <ScreenView key={stack.length} screen={top} />
    </NavContext.Provider>
  )
}

function ScreenView({ screen }: { screen: Screen }) {
  switch (screen.name) {
    case 'home':
      return <Home />
    case 'add':
      return <AddTransaction kind={screen.kind} />
    case 'report':
      return <Report month={screen.month} />
    case 'savings':
      return <Savings />
    case 'credit':
      return <Credit />
    case 'goals':
      return <Goals />
    case 'advisor':
      return (
        <Suspense fallback={<Loader />}>
          <Advisor />
        </Suspense>
      )
  }
}
