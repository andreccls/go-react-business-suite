import { screen, waitFor, within } from '@testing-library/react'
import { http } from 'msw'
import { describe, expect, it } from 'vitest'
import { API, problem } from '../test/mockApi'
import { server } from '../test/setup'
import { renderApp, startMockApi } from '../test/utils'

const open = async (opts?: Parameters<typeof renderApp>[1]) => {
  const db = startMockApi()
  const ctx = renderApp(db, { route: '/clientes', ...opts })
  await screen.findByRole('heading', { name: 'Clientes' })
  await screen.findByText('Ana Souza', { selector: 'strong' })
  return { db, ...ctx }
}

describe('customers', () => {
  it('lists customers with e-mail, formatted phone and registration date', async () => {
    await open()
    const r = screen.getByText('Ana Souza', { selector: 'strong' }).closest('tr')!
    expect(within(r).getByText('ana@example.com')).toBeInTheDocument()
    expect(within(r).getByText('(31) 99999-0000')).toBeInTheDocument()
    expect(within(r).getByText(/^\d{2}\/\d{2}\/\d{4}$/)).toBeInTheDocument()
  })

  it('searches by name or e-mail (debounced) and shows an empty state', async () => {
    const { user } = await open()
    await user.type(screen.getByLabelText('Buscar por nome ou e-mail'), 'bia@')
    await waitFor(() => expect(screen.queryByText('Ana Souza', { selector: 'strong' })).not.toBeInTheDocument())
    expect(screen.getByText('Bia Lima', { selector: 'strong' })).toBeInTheDocument()
    await user.clear(screen.getByLabelText('Buscar por nome ou e-mail'))
    await user.type(screen.getByLabelText('Buscar por nome ou e-mail'), 'zzz')
    expect(await screen.findByText('Nenhum cliente encontrado para essa busca.')).toBeInTheDocument()
  })

  it('shows the empty state of a new studio and an error with retry', async () => {
    const db = startMockApi()
    db.customers = []
    server.use(http.get(`${API}/v1/customers`, () => problem(503, 'timeout', 'slow'), { once: true }))
    const { user } = renderApp(db, { route: '/clientes' })
    expect(await screen.findByRole('alert')).toHaveTextContent(/demorou demais/)
    await user.click(screen.getByRole('button', { name: 'Tentar novamente' }))
    expect(await screen.findByText('Nenhum cliente cadastrado ainda.')).toBeInTheDocument()
  })

  it('creates a customer; the phone is sent as typed and stored as digits', async () => {
    const { user, db } = await open()
    await user.click(screen.getByRole('button', { name: 'Novo cliente' }))
    const dialog = screen.getByRole('dialog', { name: 'Novo cliente' })
    await user.type(within(dialog).getByLabelText(/Nome/), 'Carla Dias')
    await user.type(within(dialog).getByLabelText(/E-mail/), 'carla@example.com')
    await user.type(within(dialog).getByLabelText(/Telefone/), '(31) 98888-7777')
    await user.click(within(dialog).getByRole('button', { name: 'Salvar' }))
    expect(await screen.findByRole('status')).toHaveTextContent('Cliente "Carla Dias" cadastrado.')
    expect(db.customers.find((c) => c.email === 'carla@example.com')!.phone).toBe('31988887777')
    expect(await screen.findByText('(31) 98888-7777')).toBeInTheDocument()
  })

  it('validates locally (e-mail, phone) before sending', async () => {
    const { user, db } = await open()
    await user.click(screen.getByRole('button', { name: 'Novo cliente' }))
    const dialog = screen.getByRole('dialog')
    await user.type(within(dialog).getByLabelText(/Nome/), 'X')
    await user.type(within(dialog).getByLabelText(/E-mail/), 'not-an-email')
    await user.type(within(dialog).getByLabelText(/Telefone/), '123')
    await user.click(within(dialog).getByRole('button', { name: 'Salvar' }))
    expect(within(dialog).getByText(/ao menos 2/)).toBeInTheDocument()
    expect(within(dialog).getByText('Informe um e-mail válido.')).toBeInTheDocument()
    expect(within(dialog).getByText(/10 a 13 dígitos/)).toBeInTheDocument()
    expect(db.customers).toHaveLength(2)
  })

  it('puts a duplicate e-mail (409 email_taken) on the e-mail field', async () => {
    const { user } = await open()
    await user.click(screen.getByRole('button', { name: 'Novo cliente' }))
    const dialog = screen.getByRole('dialog')
    await user.type(within(dialog).getByLabelText(/Nome/), 'Outra Ana')
    await user.type(within(dialog).getByLabelText(/E-mail/), 'ana@example.com')
    await user.click(within(dialog).getByRole('button', { name: 'Salvar' }))
    expect(await within(dialog).findByText('Já existe um cadastro com esse e-mail.')).toBeInTheDocument()
    expect(within(dialog).getByLabelText(/E-mail/)).toBeInvalid()
  })

  it('shows server 422 field errors, and generic failures as a banner', async () => {
    const { user } = await open()
    server.use(http.post(`${API}/v1/customers`, () => problem(422, 'validation_failed', 'bad', [{ field: 'notes', message: 'too long' }]), { once: true }))
    await user.click(screen.getByRole('button', { name: 'Novo cliente' }))
    const dialog = screen.getByRole('dialog')
    await user.type(within(dialog).getByLabelText(/Nome/), 'Duda')
    await user.type(within(dialog).getByLabelText(/E-mail/), 'duda@example.com')
    await user.click(within(dialog).getByRole('button', { name: 'Salvar' }))
    expect(await within(dialog).findByText('too long')).toBeInTheDocument()
    server.use(http.post(`${API}/v1/customers`, () => problem(500, 'internal_error', 'x'), { once: true }))
    await user.click(within(dialog).getByRole('button', { name: 'Salvar' }))
    expect(await within(dialog).findByRole('alert')).toHaveTextContent(/Erro inesperado/)
  })

  it('edits a customer and the appointment list picks up the new name', async () => {
    const { user, db } = await open()
    await user.click(screen.getByRole('button', { name: /Editar Ana Souza/ }))
    const dialog = screen.getByRole('dialog', { name: 'Editar cliente' })
    const name = within(dialog).getByLabelText(/Nome/)
    expect(name).toHaveValue('Ana Souza')
    expect(within(dialog).getByLabelText(/Telefone/)).toHaveValue('(31) 99999-0000')
    await user.clear(name)
    await user.type(name, 'Ana Souza Lima')
    await user.click(within(dialog).getByRole('button', { name: 'Salvar' }))
    expect(await screen.findByRole('status')).toHaveTextContent('atualizado')
    expect(db.customers[0]!.name).toBe('Ana Souza Lima')
    expect(db.appointments[0]!.customer_name).toBe('Ana Souza Lima')
  })
})

describe('customers: delete is admin only', () => {
  it('staff has no delete button', async () => {
    await open({ as: 'staff' })
    expect(screen.queryByRole('button', { name: /Excluir/ })).not.toBeInTheDocument()
  })

  it('admin cannot delete a customer with appointments: 409 customer_in_use is explained', async () => {
    const { user, db } = await open()
    await user.click(screen.getByRole('button', { name: /Excluir Ana Souza/ }))
    const dialog = screen.getByRole('dialog', { name: 'Excluir cliente' })
    await user.click(within(dialog).getByRole('button', { name: 'Excluir' }))
    expect(await within(dialog).findByRole('alert')).toHaveTextContent('Este cliente tem agendamentos e não pode ser excluído.')
    expect(db.customers).toHaveLength(2)
  })

  it('admin deletes a customer without appointments', async () => {
    const { user, db } = await open()
    db.appointments = []
    await user.click(screen.getByRole('button', { name: /Excluir Bia Lima/ }))
    await user.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Excluir' }))
    expect(await screen.findByRole('status')).toHaveTextContent('Cliente "Bia Lima" excluído.')
    await waitFor(() => expect(screen.queryByText('Bia Lima', { selector: 'strong' })).not.toBeInTheDocument())
  })
})
