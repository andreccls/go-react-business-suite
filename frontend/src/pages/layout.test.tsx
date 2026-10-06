import { screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { renderApp, startMockApi } from '../test/utils'

describe('layout and permissions', () => {
  it('has a skip link, a labelled main navigation and marks the current page', async () => {
    const db = startMockApi()
    const { user } = renderApp(db)
    await screen.findByRole('heading', { name: 'Painel de resultados' })
    expect(screen.getByRole('link', { name: 'Ir para o conteúdo' })).toHaveAttribute('href', '#conteudo')
    const nav = screen.getByRole('navigation', { name: 'Principal' })
    expect(nav).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Painel' })).toHaveAttribute('aria-current', 'page')
    await user.click(screen.getByRole('link', { name: 'Agenda' }))
    expect(await screen.findByRole('heading', { name: 'Agenda' })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Agenda' })).toHaveAttribute('aria-current', 'page')
  })

  it('admin sees the Users link; staff does not, and is told off if they type the URL', async () => {
    const db = startMockApi()
    const admin = renderApp(db)
    await screen.findByRole('heading', { name: 'Painel de resultados' })
    expect(screen.getByRole('link', { name: 'Usuários' })).toBeInTheDocument()
    admin.unmount()

    const staff = renderApp(db, { as: 'staff', route: '/usuarios' })
    expect(await screen.findByRole('heading', { name: 'Acesso restrito' })).toBeInTheDocument()
    expect(screen.queryByRole('link', { name: 'Usuários' })).not.toBeInTheDocument()
    expect(screen.getByText('Equipe')).toBeInTheDocument()
    staff.unmount()
  })

  it('shows a not-found page for unknown routes', async () => {
    const db = startMockApi()
    renderApp(db, { route: '/nao-existe' })
    expect(await screen.findByRole('heading', { name: 'Página não encontrada' })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Voltar ao painel' })).toHaveAttribute('href', '/')
  })
})
