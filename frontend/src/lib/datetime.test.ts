import { describe, expect, it } from 'vitest'
import { addDays, dayOf, daysBetween, isDay, todayIn, zonedToInstant } from './datetime'

describe('dayOf / todayIn', () => {
  it('uses the business zone: 01:00 UTC is still the previous day in São Paulo', () => {
    expect(dayOf('2026-10-08T01:00:00Z')).toBe('2026-10-07')
    expect(dayOf('2026-10-08T03:00:00Z')).toBe('2026-10-08')
  })
  it('accepts Date objects and other zones', () => {
    expect(dayOf(new Date('2026-10-07T23:30:00Z'), 'Asia/Tokyo')).toBe('2026-10-08')
  })
  it('todayIn takes an injectable clock', () => {
    expect(todayIn('America/Sao_Paulo', new Date('2026-01-01T02:00:00Z'))).toBe('2025-12-31')
  })
})

describe('calendar arithmetic', () => {
  it('adds days across month, year and leap boundaries', () => {
    expect(addDays('2026-10-31', 1)).toBe('2026-11-01')
    expect(addDays('2026-01-01', -1)).toBe('2025-12-31')
    expect(addDays('2028-02-28', 1)).toBe('2028-02-29')
    expect(addDays('2026-10-07', 0)).toBe('2026-10-07')
  })
  it('rejects a malformed day', () => {
    expect(() => addDays('07/10/2026', 1)).toThrow(/invalid day/)
  })
  it('counts days between two dates', () => {
    expect(daysBetween('2026-10-01', '2026-10-31')).toBe(30)
    expect(daysBetween('2026-10-31', '2026-10-01')).toBe(-30)
    expect(daysBetween('2026-03-01', '2027-03-01')).toBe(365)
  })
  it('validates real calendar days', () => {
    expect(isDay('2026-10-07')).toBe(true)
    expect(isDay('2028-02-29')).toBe(true)
    expect(isDay('2026-02-29')).toBe(false)
    expect(isDay('2026-13-01')).toBe(false)
    expect(isDay('')).toBe(false)
    expect(isDay('2026-1-1')).toBe(false)
  })
})

describe('zonedToInstant (wall clock in a zone → UTC instant)', () => {
  it('converts São Paulo wall time (UTC-3)', () => {
    expect(zonedToInstant('2026-10-07', '14:00')).toBe('2026-10-07T17:00:00.000Z')
    expect(zonedToInstant('2026-10-07', '23:30')).toBe('2026-10-08T02:30:00.000Z')
    expect(zonedToInstant('2026-10-07', '00:00')).toBe('2026-10-07T03:00:00.000Z')
  })
  it('round-trips with dayOf', () => {
    const iso = zonedToInstant('2026-10-07', '22:15')!
    expect(dayOf(iso)).toBe('2026-10-07')
  })
  it('is right on both sides of a DST change (New York, 2026-03-08 spring forward)', () => {
    expect(zonedToInstant('2026-03-08', '01:30', 'America/New_York')).toBe('2026-03-08T06:30:00.000Z') // EST, UTC-5
    expect(zonedToInstant('2026-03-08', '03:30', 'America/New_York')).toBe('2026-03-08T07:30:00.000Z') // EDT, UTC-4
  })
  it('works for UTC and for zones ahead of UTC', () => {
    expect(zonedToInstant('2026-10-07', '10:00', 'UTC')).toBe('2026-10-07T10:00:00.000Z')
    expect(zonedToInstant('2026-10-07', '10:00', 'Asia/Tokyo')).toBe('2026-10-07T01:00:00.000Z')
  })
  it.each([
    ['2026-10-07', ''],
    ['2026-10-07', '9:00'],
    ['2026-10-07', '24:00'],
    ['2026-10-07', '12:60'],
    ['2026-02-30', '10:00'],
    ['', '10:00'],
  ])('returns null for %j %j', (day, time) => expect(zonedToInstant(day, time)).toBeNull())
})
