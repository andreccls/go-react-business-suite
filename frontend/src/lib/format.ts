import { BUSINESS_TZ } from './datetime'

const LOCALE = 'pt-BR'
const brl = new Intl.NumberFormat(LOCALE, { style: 'currency', currency: 'BRL' })
const pct = new Intl.NumberFormat(LOCALE, { style: 'percent', minimumFractionDigits: 1, maximumFractionDigits: 1 })

/** Integer cents → `R$ 1.234,56`. Money is never a float on the wire. */
export function formatMoney(cents: number): string {
  return brl.format(cents / 100)
}

const brlWhole = new Intl.NumberFormat(LOCALE, { style: 'currency', currency: 'BRL', maximumFractionDigits: 0 })

/** Integer cents → `R$ 1.234` (axis labels). */
export function formatMoneyWhole(cents: number): string {
  return brlWhole.format(cents / 100)
}

/** Ratio 0..1 → `15,8%`. */
export function formatPercent(ratio: number): string {
  return pct.format(ratio)
}

export function formatDuration(minutes: number): string {
  if (minutes < 60) return `${minutes} min`
  const h = Math.floor(minutes / 60)
  const m = minutes % 60
  return m === 0 ? `${h} h` : `${h} h ${m} min`
}

function fmt(instant: string, opts: Intl.DateTimeFormatOptions, tz: string): string {
  return new Intl.DateTimeFormat(LOCALE, { timeZone: tz, ...opts }).format(new Date(instant))
}

/** `07/10/2026` */
export function formatDate(instant: string, tz: string = BUSINESS_TZ): string {
  return fmt(instant, { day: '2-digit', month: '2-digit', year: 'numeric' }, tz)
}

/** `14:30` */
export function formatTime(instant: string, tz: string = BUSINESS_TZ): string {
  return fmt(instant, { hour: '2-digit', minute: '2-digit', hourCycle: 'h23' }, tz)
}

/** `qua., 07/10/2026 14:30` */
export function formatDateTime(instant: string, tz: string = BUSINESS_TZ): string {
  return fmt(
    instant,
    { weekday: 'short', day: '2-digit', month: '2-digit', year: 'numeric', hour: '2-digit', minute: '2-digit', hourCycle: 'h23' },
    tz,
  )
}

/** `YYYY-MM-DD` (a calendar day, already in the business zone) → `07/10`. */
export function formatDay(day: string): string {
  const [, m, d] = day.split('-')
  return `${d}/${m}`
}

/** Formats a `YYYY-MM-DD` as `07/10/2026`. */
export function formatDayFull(day: string): string {
  const [y, m, d] = day.split('-')
  return `${d}/${m}/${y}`
}

/**
 * Money typed by a person (`120`, `120,5`, `1.200,50`, `R$ 12.50`) → integer cents, or null when it
 * is not a valid amount. Parsed as text, never through floating point.
 */
export function parseMoneyToCents(input: string): number | null {
  let s = input.replace(/R\$|\s/g, '')
  if (s === '') return null
  if (s.includes(',')) s = s.replace(/\./g, '').replace(',', '.')
  const m = /^(\d+)(?:\.(\d{1,2}))?$/.exec(s)
  if (!m) return null
  return Number(m[1]) * 100 + Number((m[2] ?? '').padEnd(2, '0') || 0)
}

/** Cents → plain `120,50` for an input field. */
export function centsToInput(cents: number): string {
  return (cents / 100).toFixed(2).replace('.', ',')
}

/** `31999990000` → `(31) 99999-0000` (anything else is shown as stored). */
export function formatPhone(digits: string): string {
  const m = /^(\d{2})(\d{4,5})(\d{4})$/.exec(digits)
  return m ? `(${m[1]}) ${m[2]}-${m[3]}` : digits
}
