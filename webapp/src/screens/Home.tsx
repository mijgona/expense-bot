import { useState } from 'react'
import { useNav, useSession } from '../context'
import { getSummary, listGoals, listPayouts } from '../lib/api'
import { takePayoutLink } from '../lib/payouts'
import { useTelegramScheme } from '../lib/scheme'
import type { Payout } from '../lib/types'
import { useAsync } from '../hooks'
import { PayoutOffer, PayoutRecorded } from '../components/PayoutOffer'
import { Header } from '../components/home/Header'
import { MainCard } from '../components/home/MainCard'
import { GoalsRow } from '../components/home/GoalsRow'
import { QuickActions } from '../components/home/QuickActions'
import { LimitsCard } from '../components/home/LimitsCard'
import { DebtBlock, SavingsBlock } from '../components/home/Blocks'
import '../styles-home.css'

const samePayout = (a: Payout, month: string, kind: string) => a.month === month && a.kind === kind

/** Home screen, design B «Карточки» (feature 008). Shows only figures the app already calculates. */
export function Home() {
  const session = useSession()
  const nav = useNav()
  const scheme = useTelegramScheme()

  // Loaded in parallel with independent states; Home remounts when it becomes the top screen again,
  // so figures refresh after every write elsewhere (FR-019).
  const summary = useAsync(() => getSummary(), [])
  const payouts = useAsync(() => listPayouts(), [])
  const goals = useAsync(() => listGoals(), [])

  // Reminder deep link (?payout=YYYY-MM_kind), handed over once per app launch (007, FR-010).
  const [link, setLink] = useState(() => takePayoutLink())
  // The link may point at a month that /api/payouts (current month) doesn't cover.
  const linked = useAsync(
    () =>
      link && link.month !== session.currentMonth ? listPayouts(link.month) : Promise.resolve(null),
    [link?.month],
  )

  const items = payouts.data?.items ?? []
  const linkItem = link
    ? (items.find((p) => samePayout(p, link.month, link.kind)) ??
      linked.data?.items.find((p) => samePayout(p, link.month, link.kind)) ??
      null)
    : null
  const due = items.filter((p) => p.status === 'due' && !(linkItem && samePayout(p, linkItem.month, linkItem.kind)))

  function afterPayout() {
    setLink(null)
    summary.reload()
    payouts.reload()
  }

  const s = summary.data

  return (
    <div className="ui-b b-page" data-scheme={scheme}>
      <main className="b-main">
        <Header />

        {linkItem &&
          (linkItem.status === 'recorded' ? (
            <PayoutRecorded
              payout={linkItem}
              onOpenHistory={() => nav.push({ name: 'history', filter: { month: linkItem.month, group: 'income' } })}
            />
          ) : (
            <PayoutOffer key={`link-${linkItem.month}-${linkItem.kind}`} payout={linkItem} autoOpen onChanged={afterPayout} />
          ))}
        {due.map((p) => (
          <PayoutOffer key={`${p.month}-${p.kind}`} payout={p} onChanged={afterPayout} />
        ))}

        <MainCard
          summary={s}
          loading={summary.loading}
          error={summary.error}
          onRetry={summary.reload}
          goals={s?.quarter ? <GoalsRow quarter={s.quarter} goals={goals.data?.items ?? null} /> : undefined}
        />

        <QuickActions />

        {s && <LimitsCard lines={s.categories} />}

        {s && (
          <div className="b-blocks">
            <DebtBlock debt={s.creditDebt} />
            <SavingsBlock balance={s.savingsBalance} />
          </div>
        )}
      </main>
    </div>
  )
}
