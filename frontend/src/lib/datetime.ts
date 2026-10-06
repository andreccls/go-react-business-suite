// Calendar-day helpers in the BUSINESS time zone. The API speaks instants (RFC 3339 UTC) and
// `YYYY-MM-DD` days of the business zone; the browser's own zone is deliberately never used.

export const BUSINESS_TZ: string = import.meta.env.VITE_BUSINESS_TZ ?? 'America/Sao_Paulo'

const DAY_RE = /^(\d{4})-(\d{2})-(\d{2})$/

/** `YYYY-MM-DD` of an instant as seen in `tz`. */
export function dayOf(instant: Date | string, tz: string = BUSINESS_TZ): string {
  const d = typeof instant === 'string' ? new Date(instant) : instant
  // The sv-SE locale formats as ISO `YYYY-MM-DD`.
  return new Intl.DateTimeFormat('sv-SE', { timeZone: tz, year: 'numeric', month: '2-digit', day: '2-digit' }).format(d)
}

export function todayIn(tz: string = BUSINESS_TZ, now: Date = new Date()): string {
  return dayOf(now, tz)
}

export function isDay(s: string): boolean {
  const m = DAY_RE.exec(s)
  if (!m) return false
  const [y, mo, d] = [Number(m[1]), Number(m[2]), Number(m[3])]
  const t = new Date(Date.UTC(y, mo - 1, d))
  return t.getUTCFullYear() === y && t.getUTCMonth() === mo - 1 && t.getUTCDate() === d
}

/** Adds `n` calendar days to a `YYYY-MM-DD` string (pure date arithmetic, no zone involved). */
export function addDays(day: string, n: number): string {
  const m = DAY_RE.exec(day)
  if (!m) throw new Error(`invalid day: ${day}`)
  const t = new Date(Date.UTC(Number(m[1]), Number(m[2]) - 1, Number(m[3]) + n))
  return t.toISOString().slice(0, 10)
}

/** Whole days from `a` to `b` (b - a). */
export function daysBetween(a: string, b: string): number {
  return Math.round((Date.parse(`${b}T00:00:00Z`) - Date.parse(`${a}T00:00:00Z`)) / 86_400_000)
}

/** Offset (ms) of `tz` from UTC at the given instant: local wall clock minus UTC. */
function offsetAt(ms: number, tz: string): number {
  const parts = new Intl.DateTimeFormat('en-US', {
    timeZone: tz,
    hourCycle: 'h23',
    year: 'numeric',
    month: 'numeric',
    day: 'numeric',
    hour: 'numeric',
    minute: 'numeric',
    second: 'numeric',
  }).formatToParts(new Date(ms))
  const get = (t: string) => Number(parts.find((p) => p.type === t)?.value)
  const asUTC = Date.UTC(get('year'), get('month') - 1, get('day'), get('hour'), get('minute'), get('second'))
  return asUTC - Math.floor(ms / 1000) * 1000
}

/**
 * Wall-clock `day` + `time` (`HH:MM`) in `tz` → the UTC instant (ISO string), or null when the
 * input is malformed. Two passes make it right across DST changes.
 */
export function zonedToInstant(day: string, time: string, tz: string = BUSINESS_TZ): string | null {
  const t = /^(\d{2}):(\d{2})$/.exec(time)
  const d = DAY_RE.exec(day)
  if (!t || !d || !isDay(day)) return null
  const [h, min] = [Number(t[1]), Number(t[2])]
  if (h > 23 || min > 59) return null
  const wall = Date.UTC(Number(d[1]), Number(d[2]) - 1, Number(d[3]), h, min)
  let ms = wall - offsetAt(wall, tz)
  ms = wall - offsetAt(ms, tz)
  return new Date(ms).toISOString()
}
