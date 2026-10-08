const MONTHS = [
  'Январь', 'Февраль', 'Март', 'Апрель', 'Май', 'Июнь',
  'Июль', 'Август', 'Сентябрь', 'Октябрь', 'Ноябрь', 'Декабрь',
]

function split(key: string): [number, number] {
  const [y, m] = key.split('-').map(Number)
  return [y, m]
}

function join(y: number, m: number): string {
  return `${y}-${String(m).padStart(2, '0')}`
}

/** "2026-10" → "Октябрь 2026" */
export function monthTitle(key: string): string {
  const [y, m] = split(key)
  return `${MONTHS[m - 1] ?? key} ${y}`
}

export function prevMonth(key: string): string {
  const [y, m] = split(key)
  return m === 1 ? join(y - 1, 12) : join(y, m - 1)
}

export function nextMonth(key: string): string {
  const [y, m] = split(key)
  return m === 12 ? join(y + 1, 1) : join(y, m + 1)
}

/** Current quarter plus the next three, e.g. ["Q4 2026","Q1 2027","Q2 2027","Q3 2027"].
 *  Mirrors ledger.AllowedQuarters on the server; derived from the server's currentMonth. */
export function allowedQuarters(currentMonth: string): string[] {
  const [y, m] = split(currentMonth)
  let q = Math.floor((m - 1) / 3) + 1
  let year = y
  const out: string[] = []
  for (let i = 0; i < 4; i++) {
    out.push(`Q${q} ${year}`)
    q++
    if (q > 4) {
      q = 1
      year++
    }
  }
  return out
}

const dtf = new Intl.DateTimeFormat('ru-RU', {
  timeZone: 'Asia/Dushanbe',
  day: '2-digit',
  month: '2-digit',
  hour: '2-digit',
  minute: '2-digit',
})

/** ISO timestamp → "07.10, 14:05" in Dushanbe time. */
export function formatDateTime(iso: string): string {
  return dtf.format(new Date(iso))
}
