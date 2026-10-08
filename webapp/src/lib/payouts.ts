import type { PayoutKind } from './types'

const GENITIVE = [
  'января', 'февраля', 'марта', 'апреля', 'мая', 'июня',
  'июля', 'августа', 'сентября', 'октября', 'ноября', 'декабря',
]

/** "2026-10-15" → "15 октября" */
export function formatPayday(date: string): string {
  const [, m, d] = date.split('-').map(Number)
  return `${d} ${GENITIVE[m - 1] ?? ''}`.trim()
}

/** Short Russian title of a payment kind. */
export function payoutTitle(kind: PayoutKind): string {
  switch (kind) {
    case 'advance':
      return 'Аванс'
    case 'rest':
      return 'Зарплата (остаток)'
    case 'full':
      return 'Зарплата'
  }
}

export interface PayoutLink {
  month: string
  kind: PayoutKind
}

const LINK_RE = /^(\d{4}-\d{2})_(advance|rest|full)$/

/** Parses the reminder deep-link value "YYYY-MM_kind"; null when absent or malformed. */
export function parsePayoutLink(value: string | null | undefined): PayoutLink | null {
  const m = value ? LINK_RE.exec(value) : null
  return m ? { month: m[1], kind: m[2] as PayoutKind } : null
}

// The bot's reminder button opens WEBAPP_URL?payout=YYYY-MM_kind. Read it once at startup and hand it
// to Home exactly once, so going back to Home later doesn't reopen the confirmation.
let pending: PayoutLink | null = null

/** Captures ?payout=… from the launch URL (called once by App at startup). */
export function capturePayoutLink(): void {
  try {
    pending = parsePayoutLink(new URLSearchParams(window.location.search).get('payout'))
  } catch {
    pending = null
  }
}

/** Returns the captured deep link once, then null. */
export function takePayoutLink(): PayoutLink | null {
  const link = pending
  pending = null
  return link
}
