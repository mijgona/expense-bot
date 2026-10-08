// Money is integer diram (1 с. = 100). Never use float multiplication on amounts.

export const MAX_DIRAM = 1_000_000_000 // 10 000 000 с.

export type ParseResult = { ok: true; diram: number } | { ok: false; error: string }

/** Parses user input like "350", "350.5", "350,50", "1 234" into diram. */
export function parseAmountInput(raw: string): ParseResult {
  const s = raw.replace(/[\s ]/g, '').replace(',', '.')
  if (s === '') return { ok: false, error: 'Введите сумму' }
  if (!/^\d+(\.\d*)?$/.test(s)) {
    if (s.startsWith('-')) return { ok: false, error: 'Сумма должна быть больше нуля' }
    return { ok: false, error: 'Введите число, например 350 или 350,50' }
  }
  const [intPart, fracPart = ''] = s.split('.')
  if (fracPart.length > 2) return { ok: false, error: 'Не больше двух знаков после запятой' }
  const intDigits = intPart.replace(/^0+(?=\d)/, '')
  if (intDigits.length > 8) return { ok: false, error: 'Слишком большая сумма (максимум 10 000 000 с.)' }
  const diram = Number(intDigits) * 100 + Number(fracPart.padEnd(2, '0'))
  if (diram <= 0) return { ok: false, error: 'Сумма должна быть больше нуля' }
  if (diram > MAX_DIRAM) return { ok: false, error: 'Слишком большая сумма (максимум 10 000 000 с.)' }
  return { ok: true, diram }
}

function groupThousands(n: number): string {
  return String(n).replace(/\B(?=(\d{3})+(?!\d))/g, ' ')
}

/** Amount without the currency suffix: 765690 → "7 656,90", -50000 → "−500". */
export function formatAmount(diram: number): string {
  const neg = diram < 0
  const abs = Math.abs(Math.trunc(diram))
  const whole = Math.floor(abs / 100)
  const frac = abs % 100
  let out = groupThousands(whole)
  if (frac !== 0) out += ',' + String(frac).padStart(2, '0')
  return (neg ? '−' : '') + out
}

/** 123400 → "1 234 с.", 123450 → "1 234,50 с.", -50000 → "−500 с." */
export function formatSomoni(diram: number): string {
  return formatAmount(diram) + '\u00a0с.'
}

/** Diram → plain editable string ("1234.5" style with comma), used to prefill inputs. */
export function diramToInput(diram: number): string {
  const whole = Math.floor(diram / 100)
  const frac = diram % 100
  return frac === 0 ? String(whole) : `${whole},${String(frac).padStart(2, '0')}`
}
