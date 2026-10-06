import { fireEvent, screen, waitFor, within } from '@testing-library/react'
import { http } from 'msw'
import { describe, expect, it } from 'vitest'
import { addDays, todayIn } from '../lib/datetime'
import { API, problem } from '../test/mockApi'
import { server } from '../test/setup'
import { renderApp, startMockApi } from '../test/utils'

describe('dashboard', () => {
  it('shows the KPIs of the period, money in BRL and rates as percentages', async () => {
    const db = startMockApi()
    db.appointments[0]!.status = 'completed' // R$ 50,00 realized
    db.appointments[1]!.status = 'cancelled'
    db.appointments[1]!.starts_at = new Date(Date.now() - 26 * 3_600_000).toISOString() // inside the default period
    renderApp(db)
    const kpis = await screen.findByRole('region', { name: 'Indicadores do período' })
    expect(within(kpis).getByText('Receita realizada').nextElementSibling).toHaveTextContent(/^R\$\s50,00$/)
    expect(within(kpis).getByText('Ticket médio').nextElementSibling).toHaveTextContent(/^R\$\s50,00$/)
    expect(within(kpis).getByText('Cancelamentos').nextElementSibling).toHaveTextContent('50,0%')
    expect(within(kpis).getByText('Faltas (no-show)').nextElementSibling).toHaveTextContent('0,0%')
    expect(within(kpis).getByText('Agendamentos').nextElementSibling).toHaveTextContent('2')
    expect(within(kpis).getByText('Novos clientes').nextElementSibling).toHaveTextContent('2')
  })

  it('draws the daily chart, can switch metric, and offers the table', async () => {
    const db = startMockApi()
    db.appointments[0]!.status = 'completed'
    db.appointments[1]!.starts_at = new Date(Date.now() - 26 * 3_600_000).toISOString()
    const { user } = renderApp(db)
    const chart = await screen.findByRole('img', { name: /Receita realizada por dia/ })
    expect(chart.querySelectorAll('rect')).toHaveLength(30) // 30 days, zero-filled
    await user.click(screen.getByRole('radio', { name: 'Agendamentos' }))
    expect(screen.getByRole('img', { name: /Agendamentos por dia: total de 2/ })).toBeInTheDocument()
    expect(screen.getByRole('table', { name: 'Série diária' })).toBeInTheDocument()
  })

  it('lists top services by revenue and the upcoming appointments in the business time zone', async () => {
    const db = startMockApi()
    db.appointments[0]!.status = 'completed'
    const fixed = new Date(Date.now() + 3 * 86_400_000)
    fixed.setUTCHours(17, 0, 0, 0)
    db.appointments[1]!.starts_at = fixed.toISOString() // 17:00Z = 14:00 in São Paulo
    renderApp(db)
    const top = await screen.findByRole('region', { name: 'Top serviços' })
    expect(await within(top).findByText('Corte de cabelo')).toBeInTheDocument()
    expect(within(top).getByText('1 de 1')).toBeInTheDocument()
    const upcoming = screen.getByRole('region', { name: 'Próximos agendamentos' })
    expect(await within(upcoming).findByText('Bia Lima')).toBeInTheDocument()
    expect(within(upcoming).getByText(/14:00/)).toBeInTheDocument()
  })

  it('sends the chosen period to the API and refetches when it changes', async () => {
    const db = startMockApi()
    const seen: string[] = []
    server.events.on('request:start', ({ request }) => {
      if (request.url.includes('/dashboard/summary')) seen.push(new URL(request.url).search)
    })
    renderApp(db)
    await screen.findByRole('region', { name: 'Indicadores do período' })
    const today = todayIn()
    expect(seen[0]).toBe(`?from=${addDays(today, -29)}&to=${today}`)
    fireEvent.change(screen.getByLabelText('De'), { target: { value: addDays(today, -6) } })
    await waitFor(() => expect(seen).toContain(`?from=${addDays(today, -6)}&to=${today}`))
    server.events.removeAllListeners()
  })

  it.each([
    ['inverted', (t: string) => ({ from: addDays(t, 1), to: t }), /anterior ou igual/],
    ['too long', (t: string) => ({ from: addDays(t, -400), to: t }), /no máximo 366 dias/],
    ['empty', () => ({ from: '', to: '' }), /Informe as duas datas/],
  ])('rejects a %s period locally and hides the data', async (_name, build, message) => {
    const db = startMockApi()
    renderApp(db)
    await screen.findByRole('region', { name: 'Indicadores do período' })
    const { from, to } = build(todayIn())
    fireEvent.change(screen.getByLabelText('Até'), { target: { value: to } })
    fireEvent.change(screen.getByLabelText('De'), { target: { value: from } })
    expect(await screen.findByRole('alert')).toHaveTextContent(message)
    expect(screen.queryByRole('region', { name: 'Indicadores do período' })).not.toBeInTheDocument()
  })

  it('shows empty states when nothing happened', async () => {
    const db = startMockApi()
    db.appointments = []
    renderApp(db)
    expect(await screen.findByText('Nenhum serviço agendado no período.')).toBeInTheDocument()
    expect(await screen.findByText('Nenhum agendamento futuro.')).toBeInTheDocument()
    const kpis = screen.getByRole('region', { name: 'Indicadores do período' })
    expect(within(kpis).getByText('Receita realizada').nextElementSibling).toHaveTextContent(/^R\$\s0,00$/)
  })

  it('shows an error per card with retry instead of a blank page', async () => {
    const db = startMockApi()
    server.use(
      http.get(`${API}/v1/dashboard/summary`, () => problem(500, 'internal_error', 'x'), { once: true }),
      http.get(`${API}/v1/dashboard/daily`, () => problem(500, 'internal_error', 'x'), { once: true }),
      http.get(`${API}/v1/dashboard/top-services`, () => problem(500, 'internal_error', 'x'), { once: true }),
      http.get(`${API}/v1/dashboard/upcoming`, () => problem(500, 'internal_error', 'x'), { once: true }),
    )
    const { user } = renderApp(db)
    await waitFor(() => expect(screen.getAllByRole('button', { name: 'Tentar novamente' })).toHaveLength(4))
    for (const b of screen.getAllByRole('button', { name: 'Tentar novamente' })) await user.click(b)
    expect(await screen.findByRole('region', { name: 'Indicadores do período' })).toBeInTheDocument()
  })
})
