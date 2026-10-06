import type { Day } from '../api/types'
import { formatDay, formatDayFull, formatMoney, formatMoneyWhole } from '../lib/format'

export type Metric = 'revenue' | 'appointments'

const W = 640
const H = 240
const M = { l: 70, r: 12, t: 12, b: 28 }

/** Smallest "round" number (1, 2, 5 × 10ⁿ) ≥ v. */
export function niceCeil(v: number): number {
  if (v <= 0) return 1
  const p = 10 ** Math.floor(Math.log10(v))
  const f = v / p
  return (f <= 1 ? 1 : f <= 2 ? 2 : f <= 5 ? 5 : 10) * p
}

/** Top of the axis: 4 equal, round steps that cover `max` (and at least `min`). */
export function axisTop(max: number, min: number): number {
  return Math.max(min, 4 * niceCeil(max / 4))
}

/**
 * Daily bar chart in plain SVG (no chart library: ~80 lines instead of a dependency).
 * Accessible by construction: the SVG is an image with a text summary, each bar has a <title>
 * tooltip, and the same numbers are available as a real table below it.
 */
export function DailyChart({ data, metric }: { data: Day[]; metric: Metric }) {
  const value = (d: Day) => (metric === 'revenue' ? d.revenue_cents : d.appointments)
  const money = metric === 'revenue'
  const top = axisTop(Math.max(0, ...data.map(value)), money ? 400 : 4)
  const plotW = W - M.l - M.r
  const plotH = H - M.t - M.b
  const band = plotW / Math.max(1, data.length)
  const labelEvery = Math.max(1, Math.ceil(data.length / 8))
  const y = (v: number) => M.t + plotH - (v / top) * plotH
  const total = data.reduce((s, d) => s + value(d), 0)
  const summary = money
    ? `Receita realizada por dia: total de ${formatMoney(total)} em ${data.length} dias.`
    : `Agendamentos por dia: total de ${total} em ${data.length} dias.`

  return (
    <div>
      {/* A scrollable region must be keyboard-focusable so keyboard users can scroll it. */}
      {/* eslint-disable-next-line jsx-a11y/no-noninteractive-tabindex */}
      <div className="chart-scroll" tabIndex={0} role="region" aria-label="Gráfico diário (role para ver mais)">
        <svg className="chart-svg" viewBox={`0 0 ${W} ${H}`} role="img" aria-label={summary}>
          {[0, 1, 2, 3, 4].map((i) => {
            const v = (top / 4) * i
            return (
              <g key={i}>
                <line className="chart-grid" x1={M.l} x2={W - M.r} y1={y(v)} y2={y(v)} />
                <text className="chart-text" x={M.l - 8} y={y(v) + 4} textAnchor="end">
                  {money ? formatMoneyWhole(v) : v}
                </text>
              </g>
            )
          })}
          {data.map((d, i) => {
            const v = value(d)
            const x = M.l + i * band
            const h = Math.max(0, (v / top) * plotH)
            return (
              <g key={d.date}>
                <rect className="chart-bar" x={x + band * 0.15} y={M.t + plotH - h} width={band * 0.7} height={h} rx={2}>
                  <title>{`${formatDayFull(d.date)}: ${money ? formatMoney(v) : `${v} agendamento(s)`}`}</title>
                </rect>
                {i % labelEvery === 0 ? (
                  <text className="chart-text" x={x + band / 2} y={H - 8} textAnchor="middle">
                    {formatDay(d.date)}
                  </text>
                ) : null}
              </g>
            )
          })}
        </svg>
      </div>
      <details className="data-table">
        <summary>Ver os dados em tabela</summary>
        <div className="table-wrap">
          <table className="data">
            <caption className="sr-only">Série diária</caption>
            <thead>
              <tr>
                <th scope="col">Dia</th>
                <th scope="col" className="num">
                  Agendamentos
                </th>
                <th scope="col" className="num">
                  Concluídos
                </th>
                <th scope="col" className="num">
                  Receita
                </th>
              </tr>
            </thead>
            <tbody>
              {data.map((d) => (
                <tr key={d.date}>
                  <th scope="row">{formatDayFull(d.date)}</th>
                  <td className="num">{d.appointments}</td>
                  <td className="num">{d.completed}</td>
                  <td className="num">{formatMoney(d.revenue_cents)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </details>
    </div>
  )
}
