import { screen, waitFor, within } from '@testing-library/react'
import { http } from 'msw'
import { describe, expect, it } from 'vitest'
import type { Appointment } from '../api/types'
import { dayOf, todayIn } from '../lib/datetime'
import { formatTime } from '../lib/format'
import { API, problem, type Db } from '../test/mockApi'
import { server } from '../test/setup'
import { renderApp, startMockApi } from '../test/utils'

async function open(prepare?: (db: Db) => void, opts?: Parameters<typeof renderApp>[1]) {
  const db = startMockApi()
  prepare?.(db)
  const ctx = renderApp(db, { route: '/agenda', ...opts })
  await screen.findByRole('heading', { name: 'Agenda' })
  return { db, ...ctx }
}
const showAll = async (user: Awaited<ReturnType<typeof open>>['user']) => {
  await user.click(screen.getByRole('button', { name: 'Limpar filtros' }))
  await screen.findByText('Bia Lima', { selector: 'td' })
}
const rowOf = (name: string) => screen.getByText(name, { selector: 'td' }).closest('tr')!

describe('appointment list', () => {
  it('lists appointments with times in the business zone, price, duration and a status word', async () => {
    const fixed = new Date(Date.now() + 3 * 86_400_000)
    fixed.setUTCHours(17, 0, 0, 0)
    const { user } = await open((db) => {
      db.appointments[1]!.starts_at = fixed.toISOString()
      db.appointments[1]!.ends_at = new Date(fixed.getTime() + 3_600_000).toISOString()
    })
    await showAll(user)
    const r = rowOf('Bia Lima')
    expect(within(r).getByText(/14:00–15:00/)).toBeInTheDocument()
    expect(within(r).getByText(/R\$\s120,00/)).toBeInTheDocument()
    expect(within(r).getByText(/1 h/)).toBeInTheDocument()
    expect(within(r).getByText('Agendado')).toBeInTheDocument()
  })

  it('filters by status, and says so when nothing matches', async () => {
    const { user } = await open()
    await showAll(user)
    await user.selectOptions(screen.getByLabelText('Situação'), 'Cancelado')
    expect(await screen.findByText('Nenhum agendamento com esses filtros.')).toBeInTheDocument()
    await user.selectOptions(screen.getByLabelText('Situação'), 'Todas')
    expect(await screen.findByText('Bia Lima', { selector: 'td' })).toBeInTheDocument()
  })

  it('defaults to "from today" and the date filters narrow the list; an inverted range is flagged', async () => {
    const { user } = await open()
    expect(screen.getByLabelText('De')).toHaveValue(todayIn())
    await screen.findByText('Bia Lima', { selector: 'td' }) // the future one is there from the default filter
    await user.clear(screen.getByLabelText('De'))
    await user.type(screen.getByLabelText('De'), '2030-01-01')
    expect(await screen.findByText('Nenhum agendamento com esses filtros.')).toBeInTheDocument()
    await user.type(screen.getByLabelText('Até'), '2029-01-01')
    expect(screen.getByRole('alert')).toHaveTextContent('anterior ou igual')
  })

  it('shows an error state with retry', async () => {
    const db = startMockApi()
    server.use(http.get(`${API}/v1/appointments`, () => problem(500, 'internal_error', 'x'), { once: true }))
    const { user } = renderApp(db, { route: '/agenda' })
    expect(await screen.findByRole('alert')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Tentar novamente' }))
    expect(await screen.findByText('Bia Lima', { selector: 'td' })).toBeInTheDocument()
  })

  it('paginates', async () => {
    const { user } = await open((db) => {
      const base = db.appointments[1]!
      for (let i = 0; i < 24; i++) {
        const start = Date.parse(base.starts_at) + (i + 1) * 7_200_000
        const extra: Appointment = { ...base, id: `x${i}`, customer_name: `Cliente ${String(i).padStart(2, '0')}`, starts_at: new Date(start).toISOString(), ends_at: new Date(start + 3_600_000).toISOString() }
        db.appointments.push(extra)
      }
    })
    await showAll(user)
    expect(screen.getByText(/Página 1 de 2 · 26 registros/)).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Próxima' }))
    expect(await screen.findByText(/Página 2 de 2/)).toBeInTheDocument()
    expect(screen.getByText('Cliente 23', { selector: 'td' })).toBeInTheDocument()
  })
})

describe('status changes', () => {
  it('only offers completed / no-show once the appointment has started; cancelling is always possible', async () => {
    const { user } = await open()
    await showAll(user)
    const future = within(rowOf('Bia Lima'))
    expect(future.getByRole('button', { name: /Concluir/ })).toBeDisabled()
    expect(future.getByRole('button', { name: /Faltou/ })).toBeDisabled()
    expect(future.getByRole('button', { name: /Cancelar/ })).toBeEnabled()
    const started = within(rowOf('Ana Souza'))
    expect(started.getByRole('button', { name: /Concluir/ })).toBeEnabled()
    expect(started.getByRole('button', { name: /Faltou/ })).toBeEnabled()
  })

  it('completes an appointment that already started; it becomes terminal (no more actions)', async () => {
    const { user, db } = await open()
    await showAll(user)
    await user.click(within(rowOf('Ana Souza')).getByRole('button', { name: /Concluir/ }))
    expect(await screen.findByRole('status')).toHaveTextContent('Ana Souza: concluído.')
    expect(db.appointments[0]!.status).toBe('completed')
    await waitFor(() => expect(within(rowOf('Ana Souza')).getByText('Concluído')).toBeInTheDocument())
    expect(within(rowOf('Ana Souza')).queryByRole('button')).not.toBeInTheDocument()
  })

  it('marks a no-show', async () => {
    const { user, db } = await open()
    await showAll(user)
    await user.click(within(rowOf('Ana Souza')).getByRole('button', { name: /Faltou/ }))
    await waitFor(() => expect(db.appointments[0]!.status).toBe('no_show'))
    expect(await screen.findByText('Faltou', { selector: '.badge' })).toBeInTheDocument()
  })

  it('cancels only after the user confirms; "Voltar" keeps it', async () => {
    const { user, db } = await open()
    await showAll(user)
    await user.click(within(rowOf('Bia Lima')).getByRole('button', { name: /Cancelar/ }))
    let dialog = screen.getByRole('dialog', { name: 'Cancelar agendamento' })
    expect(dialog).toHaveTextContent('Bia Lima')
    await user.click(within(dialog).getByRole('button', { name: 'Voltar' }))
    expect(db.appointments[1]!.status).toBe('scheduled')

    await user.click(within(rowOf('Bia Lima')).getByRole('button', { name: /Cancelar/ }))
    dialog = screen.getByRole('dialog')
    await user.click(within(dialog).getByRole('button', { name: 'Cancelar agendamento' }))
    await waitFor(() => expect(db.appointments[1]!.status).toBe('cancelled'))
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    expect(await screen.findByText('Cancelado', { selector: '.badge' })).toBeInTheDocument()
  })

  it('explains a server refusal (409 not_started, e.g. a skewed clock) and refreshes the list', async () => {
    const { user } = await open()
    await showAll(user)
    server.use(http.patch(`${API}/v1/appointments/:id/status`, () => problem(409, 'not_started', 'not yet'), { once: true }))
    await user.click(within(rowOf('Ana Souza')).getByRole('button', { name: /Concluir/ }))
    expect(await screen.findByRole('alert')).toHaveTextContent('depois do horário de início')
  })

  it('explains invalid_transition when someone else already closed it, and closes the cancel dialog', async () => {
    const { user } = await open()
    await showAll(user)
    server.use(http.patch(`${API}/v1/appointments/:id/status`, () => problem(409, 'invalid_transition', 'x'), { once: true }))
    await user.click(within(rowOf('Bia Lima')).getByRole('button', { name: /Cancelar/ }))
    await user.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Cancelar agendamento' }))
    expect(await screen.findByRole('alert')).toHaveTextContent('já foi encerrado')
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  })
})

describe('booking', () => {
  const fill = async (user: Awaited<ReturnType<typeof open>>['user'], customer: string, service: string, date: string, time: string) => {
    const dialog = await screen.findByRole('dialog', { name: 'Novo agendamento' })
    await within(dialog).findByRole('option', { name: customer })
    await user.selectOptions(within(dialog).getByLabelText(/Cliente/), customer)
    await user.selectOptions(within(dialog).getByLabelText(/Serviço/), service)
    const d = within(dialog).getByLabelText(/Data/)
    await user.clear(d)
    await user.type(d, date)
    const t = within(dialog).getByLabelText(/Horário/)
    await user.clear(t)
    await user.type(t, time)
    return dialog
  }
  const tomorrow = () => {
    const d = new Date(Date.now() + 86_400_000)
    return dayOf(d)
  }

  it('books an appointment: customer + service + day/time (business zone) → shows up in the list', async () => {
    const { user, db } = await open()
    await user.click(screen.getByRole('button', { name: 'Novo agendamento' }))
    const dialog = await fill(user, 'Ana Souza', 'Massagem', tomorrow(), '10:00')
    expect(within(dialog).getByText('Termina às 11:00')).toBeInTheDocument() // 60 min service
    expect(within(dialog).getByText(/1 h · R\$\s120,00/)).toBeInTheDocument()
    await user.click(within(dialog).getByRole('button', { name: 'Agendar' }))
    expect(await screen.findByRole('status')).toHaveTextContent('Agendamento de Ana Souza (Massagem) criado.')
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    const created = db.appointments.at(-1)!
    expect(formatTime(created.starts_at)).toBe('10:00')
    expect(dayOf(created.starts_at)).toBe(tomorrow())
    expect(created).toMatchObject({ status: 'scheduled', price_cents: 12000, duration_min: 60 })
    await waitFor(() => expect(screen.getAllByText('Ana Souza', { selector: 'td' }).length).toBeGreaterThan(1))
  })

  it('only offers ACTIVE services', async () => {
    const { user } = await open()
    await user.click(screen.getByRole('button', { name: 'Novo agendamento' }))
    const dialog = await screen.findByRole('dialog')
    await within(dialog).findByRole('option', { name: 'Massagem' })
    expect(within(dialog).queryByRole('option', { name: 'Serviço antigo' })).not.toBeInTheDocument()
  })

  it('handles the overlap conflict (409 slot_unavailable): clear message, dialog stays open, nothing is created', async () => {
    const { user, db } = await open()
    const clash = db.appointments[1]!.starts_at // Bia's massage, 2 days ahead
    await user.click(screen.getByRole('button', { name: 'Novo agendamento' }))
    const dialog = await fill(user, 'Ana Souza', 'Corte de cabelo', dayOf(clash), formatTime(clash))
    await user.click(within(dialog).getByRole('button', { name: 'Agendar' }))
    expect(await within(dialog).findByRole('alert')).toHaveTextContent('Esse horário conflita com outro agendamento. Escolha outro horário.')
    expect(db.appointments).toHaveLength(2)
    expect(screen.getByRole('dialog')).toBeInTheDocument()
    // the person picks another time and tries again
    const t = within(dialog).getByLabelText(/Horário/)
    await user.clear(t)
    await user.type(t, formatTime(new Date(Date.parse(clash) + 2 * 3_600_000).toISOString()))
    await user.click(within(dialog).getByRole('button', { name: 'Agendar' }))
    await waitFor(() => expect(db.appointments).toHaveLength(3))
  })

  it('shows the API validation error for a start in the past under the time field', async () => {
    const { user } = await open()
    await user.click(screen.getByRole('button', { name: 'Novo agendamento' }))
    const dialog = await fill(user, 'Ana Souza', 'Corte de cabelo', todayIn(), '00:00')
    await user.click(within(dialog).getByRole('button', { name: 'Agendar' }))
    expect(await within(dialog).findByText('must be in the future')).toBeInTheDocument()
    expect(within(dialog).getByLabelText(/Horário/)).toBeInvalid()
  })

  it('validates locally when nothing is chosen', async () => {
    const { user, db } = await open()
    await user.click(screen.getByRole('button', { name: 'Novo agendamento' }))
    const dialog = await screen.findByRole('dialog')
    await user.click(within(dialog).getByRole('button', { name: 'Agendar' }))
    expect(within(dialog).getByText('Escolha o cliente.')).toBeInTheDocument()
    expect(within(dialog).getByText('Escolha o serviço.')).toBeInTheDocument()
    expect(db.appointments).toHaveLength(2)
  })

  it('shows other failures (service became inactive) as a banner', async () => {
    const { user, db } = await open()
    await user.click(screen.getByRole('button', { name: 'Novo agendamento' }))
    const dialog = await fill(user, 'Ana Souza', 'Corte de cabelo', tomorrow(), '16:00')
    db.services[0]!.active = false // deactivated while the form was open
    await user.click(within(dialog).getByRole('button', { name: 'Agendar' }))
    expect(await within(dialog).findByRole('alert')).toHaveTextContent('inativo')
  })

  it('tells the user when customers/services cannot be loaded', async () => {
    const db = startMockApi()
    server.use(http.get(`${API}/v1/customers`, () => problem(500, 'internal_error', 'x')))
    const { user } = renderApp(db, { route: '/agenda' })
    await screen.findByText('Bia Lima', { selector: 'td' })
    await user.click(screen.getByRole('button', { name: 'Novo agendamento' }))
    expect(await screen.findByText(/Não foi possível carregar clientes e serviços/)).toBeInTheDocument()
  })

  it('closes with Cancelar or Escape without booking', async () => {
    const { user, db } = await open()
    await user.click(screen.getByRole('button', { name: 'Novo agendamento' }))
    await screen.findByRole('dialog')
    await user.keyboard('{Escape}')
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Novo agendamento' }))
    await user.click(within(await screen.findByRole('dialog')).getByRole('button', { name: 'Cancelar' }))
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    expect(db.appointments).toHaveLength(2)
  })
})
