export { formatAmount } from '../../lib/money'

/** Russian plural: plural(5, ['день','дня','дней']) → "дней". */
export function plural(n: number, forms: [string, string, string]): string {
  const a = Math.abs(n) % 100
  const b = a % 10
  if (a > 10 && a < 20) return forms[2]
  if (b > 1 && b < 5) return forms[1]
  if (b === 1) return forms[0]
  return forms[2]
}

export const days = (n: number) => `${n} ${plural(n, ['день', 'дня', 'дней'])}`

const MONTH_ACC = [
  'январь', 'февраль', 'март', 'апрель', 'май', 'июнь',
  'июль', 'август', 'сентябрь', 'октябрь', 'ноябрь', 'декабрь',
]

/** "2026-10" → "октябрь" (as in «Остаток на октябрь»). */
export function monthWord(monthKey: string): string {
  const m = Number(monthKey.split('-')[1])
  return MONTH_ACC[m - 1] ?? ''
}

/** 0.567 → 57 */
export const pct = (share: number) => Math.round(Math.max(0, share) * 100)
