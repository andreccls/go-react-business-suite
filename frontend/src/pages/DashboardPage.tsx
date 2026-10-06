import { useState } from 'react'
import { useDaily, useSummary, useTopServices, useUpcoming } from '../api/hooks'
import type { PeriodQuery } from '../api/types'
import { DailyChart, type Metric } from '../components/DailyChart'
import { Field } from '../components/Field'
import { Empty, ErrorState, Loading } from '../components/States'
import { addDays, daysBetween, isDay, todayIn } from '../lib/datetime'
import { formatDateTime, formatMoney, formatPercent } from '../lib/format'

export function DashboardPage() {
  const today = todayIn()
  const [from, setFrom] = useState(addDays(today, -29))
  const [to, setTo] = useState(today)

  let problem: string | null = null
  if (!isDay(from) || !isDay(to)) problem = 'Informe as duas datas do período.'
  else if (from > to) problem = 'A data inicial deve ser anterior ou igual à final.'
  else if (daysBetween(from, to) > 365) problem = 'O período pode ter no máximo 366 dias.'

  return (
    <>
      <div className="page-head">
        <h1>Painel de resultados</h1>
        <form className="filters" onSubmit={(e) => e.preventDefault()} aria-label="Período">
          <Field label="De">{(c) => <input {...c} type="date" value={from} onChange={(e) => setFrom(e.target.value)} />}</Field>
          <Field label="Até">{(c) => <input {...c} type="date" value={to} onChange={(e) => setTo(e.target.value)} />}</Field>
        </form>
      </div>
      {problem ? (
        <p className="banner banner-error" role="alert">
          {problem}
        </p>
      ) : (
        <PeriodData period={{ from, to }} />
      )}
      <Upcoming />
    </>
  )
}

function PeriodData({ period }: { period: PeriodQuery }) {
  return (
    <>
      <Kpis period={period} />
      <div className="grid-2">
        <DailyCard period={period} />
        <TopServicesCard period={period} />
      </div>
    </>
  )
}

function Kpis({ period }: { period: PeriodQuery }) {
  const q = useSummary(period)
  if (q.isPending) return <Loading label="Calculando indicadores…" />
  if (q.isError) return <ErrorState error={q.error} onRetry={() => void q.refetch()} />
  const s = q.data
  return (
    <section aria-label="Indicadores do período" className="kpis">
      <div className="kpi">
        <div className="kpi-label">Receita realizada</div>
        <div className="kpi-value">{formatMoney(s.revenue_cents)}</div>
        <div className="kpi-sub">{s.by_status.completed} concluídos</div>
      </div>
      <div className="kpi">
        <div className="kpi-label">Agendamentos</div>
        <div className="kpi-value">{s.appointments_total}</div>
        <div className="kpi-sub">
          {s.by_status.scheduled} agendados · {s.by_status.cancelled} cancelados · {s.by_status.no_show} faltas
        </div>
      </div>
      <div className="kpi">
        <div className="kpi-label">Ticket médio</div>
        <div className="kpi-value">{formatMoney(s.average_ticket_cents)}</div>
        <div className="kpi-sub">por atendimento concluído</div>
      </div>
      <div className="kpi">
        <div className="kpi-label">Cancelamentos</div>
        <div className="kpi-value">{formatPercent(s.cancellation_rate)}</div>
        <div className="kpi-sub">{s.by_status.cancelled} no período</div>
      </div>
      <div className="kpi">
        <div className="kpi-label">Faltas (no-show)</div>
        <div className="kpi-value">{formatPercent(s.no_show_rate)}</div>
        <div className="kpi-sub">{s.by_status.no_show} no período</div>
      </div>
      <div className="kpi">
        <div className="kpi-label">Novos clientes</div>
        <div className="kpi-value">{s.new_customers}</div>
        <div className="kpi-sub">cadastrados no período</div>
      </div>
    </section>
  )
}

function DailyCard({ period }: { period: PeriodQuery }) {
  const q = useDaily(period)
  const [metric, setMetric] = useState<Metric>('revenue')
  return (
    <section className="card" aria-labelledby="daily-title">
      <div className="chart-head">
        <h2 id="daily-title">Série diária</h2>
        <fieldset className="seg">
          <legend className="sr-only">Métrica do gráfico</legend>
          <label>
            <input type="radio" name="metric" checked={metric === 'revenue'} onChange={() => setMetric('revenue')} />
            <span>Receita</span>
          </label>
          <label>
            <input type="radio" name="metric" checked={metric === 'appointments'} onChange={() => setMetric('appointments')} />
            <span>Agendamentos</span>
          </label>
        </fieldset>
      </div>
      {q.isPending ? <Loading /> : q.isError ? <ErrorState error={q.error} onRetry={() => void q.refetch()} /> : <DailyChart data={q.data.data} metric={metric} />}
    </section>
  )
}

function TopServicesCard({ period }: { period: PeriodQuery }) {
  const q = useTopServices(period)
  return (
    <section className="card" aria-labelledby="top-title">
      <h2 id="top-title">Top serviços</h2>
      {q.isPending ? (
        <Loading />
      ) : q.isError ? (
        <ErrorState error={q.error} onRetry={() => void q.refetch()} />
      ) : q.data.data.length === 0 ? (
        <Empty>Nenhum serviço agendado no período.</Empty>
      ) : (
        <div className="table-wrap">
          <table className="data stack">
            <thead>
              <tr>
                <th scope="col">Serviço</th>
                <th scope="col" className="num">
                  Concluídos
                </th>
                <th scope="col" className="num">
                  Receita
                </th>
              </tr>
            </thead>
            <tbody>
              {q.data.data.map((t) => (
                <tr key={t.service_id}>
                  <td data-label="Serviço">{t.name}</td>
                  <td data-label="Concluídos" className="num">
                    {t.completed} de {t.appointments}
                  </td>
                  <td data-label="Receita" className="num">
                    {formatMoney(t.revenue_cents)}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </section>
  )
}

function Upcoming() {
  const q = useUpcoming()
  return (
    <section className="card" aria-labelledby="up-title">
      <h2 id="up-title">Próximos agendamentos</h2>
      {q.isPending ? (
        <Loading />
      ) : q.isError ? (
        <ErrorState error={q.error} onRetry={() => void q.refetch()} />
      ) : q.data.data.length === 0 ? (
        <Empty>Nenhum agendamento futuro.</Empty>
      ) : (
        <div className="table-wrap">
          <table className="data stack">
            <thead>
              <tr>
                <th scope="col">Quando</th>
                <th scope="col">Cliente</th>
                <th scope="col">Serviço</th>
              </tr>
            </thead>
            <tbody>
              {q.data.data.map((a) => (
                <tr key={a.id}>
                  <td data-label="Quando">{formatDateTime(a.starts_at)}</td>
                  <td data-label="Cliente">{a.customer_name}</td>
                  <td data-label="Serviço">{a.service_name}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </section>
  )
}
