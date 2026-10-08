import { lazy, Suspense, useCallback, useMemo, useState } from 'react'
import {
  NavContext,
  SessionContext,
  SessionReloadContext,
  useSession,
  type Nav,
  type Screen,
  type Tab,
} from './context'
import { ApiError, getSession } from './lib/api'
import { isInsideTelegram, useBackButton } from './lib/telegram'
import { capturePayoutLink } from './lib/payouts'
import { useAsync } from './hooks'
import { Loader } from './components/Loader'
import { ErrorBanner } from './components/ErrorBanner'
import { OutsideTelegram } from './screens/OutsideTelegram'
import { Denied, Reopen } from './screens/Denied'
import { AddTransaction } from './screens/AddTransaction'
import { Report } from './screens/Report'
import { Savings } from './screens/Savings'
import { Credit } from './screens/Credit'
import { Goals } from './screens/Goals'
import { TabBar } from './components/TabBar'

// The redesigned home screen (008) brings its own stylesheet and fonts; keep it out of the first-load chunk.
const Home = lazy(() => import('./screens/Home').then((m) => ({ default: m.Home })))
// Advisor pulls in react-markdown (~45 KB gz); load it only when the screen is opened.
const Advisor = lazy(() => import('./screens/Advisor').then((m) => ({ default: m.Advisor })))
// History / edit / profile are secondary screens; split them out to keep the first-load chunk small.
const History = lazy(() => import('./screens/History').then((m) => ({ default: m.History })))
const EditTransaction = lazy(() => import('./screens/EditTransaction').then((m) => ({ default: m.EditTransaction })))
const Profile = lazy(() => import('./screens/Profile').then((m) => ({ default: m.Profile })))
const Categories = lazy(() => import('./screens/Categories').then((m) => ({ default: m.Categories })))

// Reminder button opens WEBAPP_URL?payout=YYYY-MM_kind; Home opens that payment's confirmation (FR-012).
capturePayoutLink()

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
      <SessionReloadContext.Provider value={session.reload}>
        <Navigator />
      </SessionReloadContext.Provider>
    </SessionContext.Provider>
  )
}

function Navigator() {
  const session = useSession()
  // Tab roots (Главная, История, Отчёт, Профиль) + a stack of screens opened on top (008 FR-011).
  // The reminder deep link (?payout=…) is consumed by Home, which is the initial tab.
  const [tab, setTabState] = useState<Tab>('home')
  const [stack, setStack] = useState<Screen[]>([])
  const push = useCallback((s: Screen) => setStack((st) => [...st, s]), [])
  const pop = useCallback(() => setStack((st) => st.slice(0, -1)), [])
  const setTab = useCallback(
    (t: Tab) => {
      if (t === tab) return // tapping the active tab does nothing
      setTabState(t)
      setStack([])
    },
    [tab],
  )
  const nav = useMemo<Nav>(() => ({ push, pop, tab, setTab }), [push, pop, tab, setTab])

  useBackButton(stack.length > 0 ? pop : null)

  const atRoot = stack.length === 0
  const top: Screen = stack[stack.length - 1] ?? tabRoot(tab, session.currentMonth)
  // Only the top screen is mounted, so a screen refetches its data whenever it becomes visible again.
  const view = <ScreenView key={`${tab}-${stack.length}`} screen={top} />
  return (
    <NavContext.Provider value={nav}>
      {atRoot && tab !== 'home' ? <div className="with-tabbar">{view}</div> : view}
      {atRoot && <TabBar />}
    </NavContext.Provider>
  )
}

/** Root screen of a tab: Report opens the current month, History opens without filters. */
function tabRoot(tab: Tab, currentMonth: string): Screen {
  switch (tab) {
    case 'home':
      return { name: 'home' }
    case 'history':
      return { name: 'history' }
    case 'report':
      return { name: 'report', month: currentMonth }
    case 'profile':
      return { name: 'profile' }
  }
}

function ScreenView({ screen }: { screen: Screen }) {
  switch (screen.name) {
    case 'home':
      return (
        <Suspense fallback={<Loader />}>
          <Home />
        </Suspense>
      )
    case 'add':
      return <AddTransaction kind={screen.kind} />
    case 'report':
      return <Report month={screen.month} />
    case 'savings':
      return <Savings initial={screen.mode} />
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
    case 'history':
      return (
        <Suspense fallback={<Loader />}>
          <History filter={screen.filter} />
        </Suspense>
      )
    case 'edit':
      return (
        <Suspense fallback={<Loader />}>
          <EditTransaction tx={screen.tx} />
        </Suspense>
      )
    case 'profile':
      return (
        <Suspense fallback={<Loader />}>
          <Profile />
        </Suspense>
      )
    case 'categories':
      return (
        <Suspense fallback={<Loader />}>
          <Categories />
        </Suspense>
      )
  }
}
