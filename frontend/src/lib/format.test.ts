import { describe, expect, it } from 'vitest'
import {
  centsToInput,
  formatDate,
  formatDateTime,
  formatDay,
  formatDayFull,
  formatDuration,
  formatMoney,
  formatMoneyWhole,
  formatPercent,
  formatPhone,
  formatTime,
  parseMoneyToCents,
} from './format'

// Intl uses a no-break space between "R$" and the number.
const plain = (s: string) => s.replace(/\u00a0/g, ' ')

describe('formatMoney', () => {
  it.each([
    [0, 'R$ 0,00'],
    [5, 'R$ 0,05'],
    [12000, 'R$ 120,00'],
    [119000, 'R$ 1.190,00'],
    [10818, 'R$ 108,18'],
    [10_000_000, 'R$ 100.000,00'],
  ])('%i cents → %s', (cents, text) => {
    expect(plain(formatMoney(cents))).toBe(text)
  })

  it('formats whole reais for axis labels', () => {
    expect(plain(formatMoneyWhole(125000))).toBe('R$ 1.250')
  })
})

describe('formatPercent / formatDuration / formatPhone', () => {
  it('shows ratios with one decimal and a comma', () => {
    expect(plain(formatPercent(0.1579))).toBe('15,8%')
    expect(plain(formatPercent(0))).toBe('0,0%')
  })
  it.each([
    [5, '5 min'],
    [45, '45 min'],
    [60, '1 h'],
    [90, '1 h 30 min'],
    [480, '8 h'],
  ])('%i min → %s', (m, text) => expect(formatDuration(m)).toBe(text))
  it('formats stored phone digits, leaves odd values alone', () => {
    expect(formatPhone('31999990000')).toBe('(31) 99999-0000')
    expect(formatPhone('3133334444')).toBe('(31) 3333-4444')
    expect(formatPhone('123')).toBe('123')
  })
})

describe('dates are shown in the business time zone, not the browser one', () => {
  it('converts a UTC instant to America/Sao_Paulo (UTC-3)', () => {
    expect(formatDate('2026-10-07T17:00:00Z')).toBe('07/10/2026')
    expect(formatTime('2026-10-07T17:00:00Z')).toBe('14:00')
  })
  it('keeps the previous calendar day when the UTC date has already rolled over', () => {
    expect(formatDate('2026-10-08T02:30:00Z')).toBe('07/10/2026')
    expect(formatTime('2026-10-08T02:30:00Z')).toBe('23:30')
  })
  it('honours an explicit zone and a 24h clock at midnight', () => {
    expect(formatTime('2026-10-07T00:05:00Z', 'UTC')).toBe('00:05')
    expect(formatDate('2026-10-07T00:05:00Z', 'Asia/Tokyo')).toBe('07/10/2026')
  })
  it('builds a readable date-time with weekday', () => {
    expect(formatDateTime('2026-10-07T17:00:00Z')).toMatch(/qua\.?,? 07\/10\/2026,? 14:00/)
  })
  it('formats calendar days without touching zones', () => {
    expect(formatDay('2026-10-07')).toBe('07/10')
    expect(formatDayFull('2026-10-07')).toBe('07/10/2026')
  })
})

describe('parseMoneyToCents', () => {
  it.each([
    ['120', 12000],
    ['120,5', 12050],
    ['120,50', 12050],
    ['120.50', 12050],
    ['1.200,50', 120050],
    ['R$ 12,00', 1200],
    ['0,05', 5],
    ['0', 0],
    ['  7 ', 700],
  ])('%j → %i', (input, cents) => expect(parseMoneyToCents(input)).toBe(cents))

  it.each([[''], ['   '], ['abc'], ['12,345'], ['-5'], ['1,2,3'], ['12,'], [',5']])('%j is not a valid amount', (input) => {
    expect(parseMoneyToCents(input)).toBeNull()
  })

  it('never goes through floating point (19,90 would be 1989.999… as a float)', () => {
    expect(parseMoneyToCents('19,90')).toBe(1990)
    expect(parseMoneyToCents('0,29')).toBe(29)
  })

  it('round-trips with centsToInput', () => {
    for (const cents of [0, 5, 100, 12050, 120050]) expect(parseMoneyToCents(centsToInput(cents))).toBe(cents)
  })
})
