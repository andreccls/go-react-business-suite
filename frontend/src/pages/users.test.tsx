import { screen } from '@testing-library/react'
import { http } from 'msw'
import { describe, expect, it } from 'vitest'
import { API, problem } from '../test/mockApi'
import { server } from '../test/setup'
import { renderApp, startMockApi } from '../test/utils'

const open = async () => {
  const db = startMockApi()
  const ctx = renderApp(db, { route: '/usuarios' })
  await screen.findByRole('heading', { name: 'Novo usuário' })
  return { db, ...ctx }
}

describe('users (admin)', () => {
  it('creates a staff user and lists it as created in this session', async () => {
    const { user, db } = await open()
    expect(screen.getByText(/não oferece listagem de usuários/)).toBeInTheDocument()
    await user.type(screen.getByLabelText(/E-mail/), 'recepcao@example.com')
    await user.type(screen.getByLabelText(/Senha inicial/), 'uma-senha-longa')
    await user.click(screen.getByRole('button', { name: 'Criar usuário' }))
    expect(await screen.findByText(/recepcao@example\.com/)).toBeInTheDocument()
    expect(db.accounts.find((a) => a.email === 'recepcao@example.com')).toMatchObject({ role: 'staff' })
    expect(screen.getByLabelText(/E-mail/)).toHaveValue('') // form reset
  })

  it('can create an admin too', async () => {
    const { user, db } = await open()
    await user.type(screen.getByLabelText(/E-mail/), 'chefe@example.com')
    await user.type(screen.getByLabelText(/Senha inicial/), 'uma-senha-longa')
    await user.selectOptions(screen.getByLabelText(/Papel/), 'Administrador')
    await user.click(screen.getByRole('button', { name: 'Criar usuário' }))
    expect(await screen.findByText(/chefe@example\.com/)).toBeInTheDocument()
    expect(db.accounts.at(-1)!.role).toBe('admin')
  })

  it('validates e-mail and password length locally', async () => {
    const { user, db } = await open()
    await user.type(screen.getByLabelText(/E-mail/), 'nope')
    await user.type(screen.getByLabelText(/Senha inicial/), '123')
    await user.click(screen.getByRole('button', { name: 'Criar usuário' }))
    expect(screen.getByText('Informe um e-mail válido.')).toBeInTheDocument()
    expect(screen.getByText('A senha deve ter de 8 a 72 caracteres.')).toBeInTheDocument()
    expect(db.accounts).toHaveLength(2)
  })

  it('reports a taken e-mail on the field, API validation errors, and other failures', async () => {
    const { user } = await open()
    await user.type(screen.getByLabelText(/E-mail/), 'staff@example.com')
    await user.type(screen.getByLabelText(/Senha inicial/), 'uma-senha-longa')
    await user.click(screen.getByRole('button', { name: 'Criar usuário' }))
    expect(await screen.findByText('Já existe um cadastro com esse e-mail.')).toBeInTheDocument()

    server.use(http.post(`${API}/v1/users`, () => problem(422, 'validation_failed', 'bad', [{ field: 'password', message: 'too common' }]), { once: true }))
    await user.click(screen.getByRole('button', { name: 'Criar usuário' }))
    expect(await screen.findByText('too common')).toBeInTheDocument()

    server.use(http.post(`${API}/v1/users`, () => problem(500, 'internal_error', 'x'), { once: true }))
    await user.click(screen.getByRole('button', { name: 'Criar usuário' }))
    expect(await screen.findByRole('alert')).toHaveTextContent(/Erro inesperado/)
  })
})
