import { screen, waitFor, within } from '@testing-library/react'
import { http } from 'msw'
import { describe, expect, it } from 'vitest'
import { API, problem } from '../test/mockApi'
import { server } from '../test/setup'
import { renderApp, startMockApi } from '../test/utils'

const open = async (opts?: Parameters<typeof renderApp>[1]) => {
  const db = startMockApi()
  const ctx = renderApp(db, { route: '/servicos', ...opts })
  await screen.findByRole('heading', { name: 'Serviços' })
  return { db, ...ctx }
}
const row = (name: string) => screen.getByText(name, { selector: 'strong' }).closest('tr')!

describe('services list', () => {
  it('shows name, duration, BRL price from cents and status', async () => {
    await open()
    await screen.findByText('Corte de cabelo', { selector: 'strong' })
    const r = row('Corte de cabelo')
    expect(await within(r).findByText('30 min')).toBeInTheDocument()
    expect(within(r).getByText(/R\$\s50,00/)).toBeInTheDocument()
    expect(within(r).getByText('Ativo')).toBeInTheDocument()
    expect(within(row('Massagem')).getByText('1 h')).toBeInTheDocument()
    expect(within(row('Serviço antigo')).getByText('Inativo')).toBeInTheDocument()
  })

  it('shows an empty state, with a different message when filters are on', async () => {
    const db = startMockApi()
    db.services = []
    const { user } = renderApp(db, { route: '/servicos' })
    expect(await screen.findByText('Nenhum serviço cadastrado ainda.')).toBeInTheDocument()
    await user.type(screen.getByLabelText('Buscar por nome'), 'xyz')
    expect(await screen.findByText('Nenhum serviço encontrado com esses filtros.')).toBeInTheDocument()
  })

  it('shows an error state and recovers on retry', async () => {
    const db = startMockApi()
    server.use(http.get(`${API}/v1/services`, () => problem(500, 'internal_error', 'boom'), { once: true }))
    const { user } = renderApp(db, { route: '/servicos' })
    expect(await screen.findByRole('alert')).toHaveTextContent(/Erro inesperado/)
    await user.click(screen.getByRole('button', { name: 'Tentar novamente' }))
    expect(await screen.findByText('Corte de cabelo', { selector: 'strong' })).toBeInTheDocument()
  })

  it('filters by name and by situation', async () => {
    const { user } = await open()
    await screen.findByText('Corte de cabelo', { selector: 'strong' })
    await user.type(screen.getByLabelText('Buscar por nome'), 'mass')
    await waitFor(() => expect(screen.queryByText('Corte de cabelo', { selector: 'strong' })).not.toBeInTheDocument())
    expect(screen.getByText('Massagem', { selector: 'strong' })).toBeInTheDocument()
    await user.clear(screen.getByLabelText('Buscar por nome'))
    await user.selectOptions(screen.getByLabelText('Situação'), 'Inativos')
    await waitFor(() => expect(screen.queryByText('Massagem', { selector: 'strong' })).not.toBeInTheDocument())
    expect(await screen.findByText('Serviço antigo', { selector: 'strong' })).toBeInTheDocument()
  })
})

describe('services CRUD', () => {
  it('creates a service: parses "49,90" into 4990 cents and lists it', async () => {
    const { user, db } = await open()
    await user.click(screen.getByRole('button', { name: 'Novo serviço' }))
    const dialog = screen.getByRole('dialog', { name: 'Novo serviço' })
    await user.type(within(dialog).getByLabelText(/Nome/), 'Manicure')
    await user.clear(within(dialog).getByLabelText(/Duração/))
    await user.type(within(dialog).getByLabelText(/Duração/), '45')
    await user.type(within(dialog).getByLabelText(/Preço/), '49,90')
    await user.click(within(dialog).getByRole('button', { name: 'Salvar' }))
    expect(await screen.findByRole('status')).toHaveTextContent('Serviço "Manicure" criado.')
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    expect(db.services.find((s) => s.name === 'Manicure')).toMatchObject({ duration_min: 45, price_cents: 4990, active: true })
    expect(await screen.findByText('Manicure', { selector: 'strong' })).toBeInTheDocument()
  })

  it('validates in the browser first: nothing is sent and each field explains itself', async () => {
    const { user, db } = await open()
    const before = db.services.length
    await user.click(screen.getByRole('button', { name: 'Novo serviço' }))
    const dialog = screen.getByRole('dialog')
    await user.clear(within(dialog).getByLabelText(/Duração/))
    await user.click(within(dialog).getByRole('button', { name: 'Salvar' }))
    expect(within(dialog).getByText('Nome é obrigatório.')).toBeInTheDocument()
    expect(within(dialog).getByText(/duração em minutos/)).toBeInTheDocument()
    expect(within(dialog).getByText(/preço válido/)).toBeInTheDocument()
    expect(within(dialog).getByLabelText(/Nome/)).toBeInvalid()
    expect(db.services).toHaveLength(before)
  })

  it('shows the API 422 field errors under the right field', async () => {
    const { user } = await open()
    server.use(
      http.post(`${API}/v1/services`, () => problem(422, 'validation_failed', 'invalid', [{ field: 'price_cents', message: 'must be at most 10000000' }])),
    )
    await user.click(screen.getByRole('button', { name: 'Novo serviço' }))
    const dialog = screen.getByRole('dialog')
    await user.type(within(dialog).getByLabelText(/Nome/), 'Caro')
    await user.type(within(dialog).getByLabelText(/Preço/), '10')
    await user.click(within(dialog).getByRole('button', { name: 'Salvar' }))
    expect(await within(dialog).findByText('must be at most 10000000')).toBeInTheDocument()
    expect(within(dialog).getByLabelText(/Preço/)).toBeInvalid()
  })

  it('shows a generic server failure as a banner and keeps the dialog open', async () => {
    const { user } = await open()
    server.use(http.post(`${API}/v1/services`, () => problem(500, 'internal_error', 'boom')))
    await user.click(screen.getByRole('button', { name: 'Novo serviço' }))
    const dialog = screen.getByRole('dialog')
    await user.type(within(dialog).getByLabelText(/Nome/), 'Falha')
    await user.type(within(dialog).getByLabelText(/Preço/), '10')
    await user.click(within(dialog).getByRole('button', { name: 'Salvar' }))
    expect(await within(dialog).findByRole('alert')).toHaveTextContent(/Erro inesperado/)
  })

  it('edits a service (form is pre-filled, price shown in BRL format)', async () => {
    const { user, db } = await open()
    await screen.findByText('Massagem', { selector: 'strong' })
    await user.click(screen.getByRole('button', { name: 'Editar Massagem' }))
    const dialog = screen.getByRole('dialog', { name: 'Editar serviço' })
    const price = within(dialog).getByLabelText(/Preço/)
    expect(price).toHaveValue('120,00')
    await user.clear(price)
    await user.type(price, '130')
    await user.click(within(dialog).getByRole('button', { name: 'Salvar' }))
    expect(await screen.findByRole('status')).toHaveTextContent('atualizado')
    expect(db.services.find((s) => s.id === 's2')!.price_cents).toBe(13000)
    expect(await screen.findByText(/R\$\s130,00/)).toBeInTheDocument()
  })

  it('activates and deactivates', async () => {
    const { user } = await open()
    await screen.findByText('Corte de cabelo', { selector: 'strong' })
    await user.click(screen.getByRole('button', { name: /Desativar Corte de cabelo/ }))
    expect(await screen.findByRole('status')).toHaveTextContent('"Corte de cabelo" desativado.')
    await waitFor(() => expect(within(row('Corte de cabelo')).getByText('Inativo')).toBeInTheDocument())
    await user.click(screen.getByRole('button', { name: /^Ativar Corte de cabelo/ }))
    await waitFor(() => expect(within(row('Corte de cabelo')).getByText('Ativo')).toBeInTheDocument())
  })

  it('shows a failed toggle as an error', async () => {
    const { user } = await open()
    await screen.findByText('Corte de cabelo', { selector: 'strong' })
    server.use(http.patch(`${API}/v1/services/:id`, () => problem(404, 'service_not_found', 'gone')))
    await user.click(screen.getByRole('button', { name: /Desativar Corte de cabelo/ }))
    expect(await screen.findByRole('alert')).toHaveTextContent(/não encontrado/)
  })
})

describe('services: delete is admin only', () => {
  it('staff does not even see the delete button', async () => {
    await open({ as: 'staff' })
    await screen.findByText('Corte de cabelo', { selector: 'strong' })
    expect(screen.queryByRole('button', { name: /Excluir/ })).not.toBeInTheDocument()
    expect(screen.getAllByRole('button', { name: /Editar/ }).length).toBeGreaterThan(0)
  })

  it('admin deletes an unused service after confirming', async () => {
    const { user, db } = await open()
    await screen.findByText('Serviço antigo', { selector: 'strong' })
    await user.click(screen.getByRole('button', { name: /Excluir Serviço antigo/ }))
    const dialog = screen.getByRole('dialog', { name: 'Excluir serviço' })
    await user.click(within(dialog).getByRole('button', { name: 'Excluir' }))
    expect(await screen.findByRole('status')).toHaveTextContent('excluído')
    expect(db.services.some((s) => s.id === 's3')).toBe(false)
    await waitFor(() => expect(screen.queryByText('Serviço antigo', { selector: 'strong' })).not.toBeInTheDocument())
  })

  it('warns when the service is in use (409 service_in_use) and keeps it', async () => {
    const { user, db } = await open()
    await screen.findByText('Corte de cabelo', { selector: 'strong' })
    await user.click(screen.getByRole('button', { name: /Excluir Corte de cabelo/ }))
    const dialog = screen.getByRole('dialog')
    await user.click(within(dialog).getByRole('button', { name: 'Excluir' }))
    expect(await within(dialog).findByRole('alert')).toHaveTextContent('tem agendamentos e não pode ser excluído. Desative-o')
    expect(db.services.some((s) => s.id === 's1')).toBe(true)
    await user.click(within(dialog).getByRole('button', { name: 'Voltar' }))
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  })

  it('a 403 from the API (role changed meanwhile) is shown, not swallowed', async () => {
    const { user } = await open()
    await screen.findByText('Serviço antigo', { selector: 'strong' })
    server.use(http.delete(`${API}/v1/services/:id`, () => problem(403, 'forbidden', 'Admin only')))
    await user.click(screen.getByRole('button', { name: /Excluir Serviço antigo/ }))
    await user.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Excluir' }))
    expect(await screen.findByRole('alert')).toHaveTextContent('permissão')
  })
})
