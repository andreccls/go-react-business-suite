import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { describe, expect, it, vi } from 'vitest'
import type { Day } from '../api/types'
import { ApiError } from '../api/errors'
import { ConfirmDialog } from './ConfirmDialog'
import { axisTop, DailyChart, niceCeil } from './DailyChart'
import { Field } from './Field'
import { Modal } from './Modal'
import { Pagination } from './Pagination'
import { Empty, ErrorState, Loading } from './States'
import { StatusBadge } from './StatusBadge'

describe('Field', () => {
  it('wires label, hint and error to the control for screen readers', () => {
    render(
      <Field label="Nome" hint="Como no documento" error="Obrigatório" required>
        {(c) => <input {...c} />}
      </Field>,
    )
    const input = screen.getByLabelText(/Nome/)
    expect(input).toBeRequired()
    expect(input).toBeInvalid()
    expect(input).toHaveAccessibleDescription('Como no documento Obrigatório')
  })
  it('is valid and undescribed without hint or error', () => {
    render(<Field label="Idade">{(c) => <input {...c} />}</Field>)
    const input = screen.getByLabelText('Idade')
    expect(input).toBeValid()
    expect(input).not.toHaveAttribute('aria-describedby')
  })
})

describe('Modal', () => {
  function Harness({ onClose = () => {} }: { onClose?: () => void }) {
    const [open, setOpen] = useState(false)
    return (
      <>
        <button onClick={() => setOpen(true)}>abrir</button>
        {open ? (
          <Modal
            title="Título"
            onClose={() => {
              onClose()
              setOpen(false)
            }}
          >
            <input aria-label="primeiro" />
            <button>último</button>
          </Modal>
        ) : null}
      </>
    )
  }

  it('is a labelled modal dialog, focuses its first field, closes on Escape and gives focus back', async () => {
    const user = userEvent.setup()
    const onClose = vi.fn()
    render(<Harness onClose={onClose} />)
    const opener = screen.getByRole('button', { name: 'abrir' })
    await user.click(opener)
    const dialog = screen.getByRole('dialog', { name: 'Título' })
    expect(dialog).toHaveAttribute('aria-modal', 'true')
    expect(screen.getByLabelText('primeiro')).toHaveFocus()
    await user.keyboard('{Escape}')
    expect(onClose).toHaveBeenCalledTimes(1)
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    expect(opener).toHaveFocus()
  })

  it('keeps Tab and Shift+Tab inside the dialog', async () => {
    const user = userEvent.setup()
    render(<Harness />)
    await user.click(screen.getByRole('button', { name: 'abrir' }))
    const first = screen.getByLabelText('primeiro')
    const last = screen.getByRole('button', { name: 'último' })
    await user.tab()
    expect(last).toHaveFocus()
    await user.tab()
    expect(first).toHaveFocus() // wrapped around instead of leaving the dialog
    await user.tab({ shift: true })
    expect(last).toHaveFocus()
  })

  it('closes when the backdrop is clicked, not when the dialog itself is', async () => {
    const user = userEvent.setup()
    const onClose = vi.fn()
    render(
      <Modal title="T" onClose={onClose}>
        <p>corpo</p>
      </Modal>,
    )
    await user.click(screen.getByText('corpo'))
    expect(onClose).not.toHaveBeenCalled()
    await user.click(screen.getByRole('dialog').parentElement!)
    expect(onClose).toHaveBeenCalledTimes(1)
  })

  it('focuses the dialog itself when it has nothing focusable', () => {
    render(
      <Modal title="Vazio" onClose={() => {}}>
        <p>só texto</p>
      </Modal>,
    )
    expect(screen.getByRole('dialog')).toHaveFocus()
  })
})

describe('ConfirmDialog', () => {
  it('confirms, cancels and shows a failure message from the API', async () => {
    const user = userEvent.setup()
    const onConfirm = vi.fn()
    const onCancel = vi.fn()
    render(
      <ConfirmDialog title="Excluir" message="Tem certeza?" confirmLabel="Excluir" error={new ApiError({ status: 409, code: 'service_in_use' })} onConfirm={onConfirm} onCancel={onCancel} />,
    )
    expect(screen.getByRole('alert')).toHaveTextContent(/Desative-o/)
    await user.click(screen.getByRole('button', { name: 'Excluir' }))
    await user.click(screen.getByRole('button', { name: 'Voltar' }))
    expect(onConfirm).toHaveBeenCalledOnce()
    expect(onCancel).toHaveBeenCalledOnce()
  })
  it('disables confirm while busy', () => {
    render(<ConfirmDialog title="X" message="m" confirmLabel="Sim" busy onConfirm={() => {}} onCancel={() => {}} />)
    expect(screen.getByRole('button', { name: 'Aguarde…' })).toBeDisabled()
  })
})

describe('Pagination', () => {
  it('shows only the count when everything fits on one page', () => {
    render(<Pagination page={1} pageSize={20} total={1} onChange={() => {}} />)
    expect(screen.getByText('1 registro')).toBeInTheDocument()
    expect(screen.queryByRole('navigation')).not.toBeInTheDocument()
    render(<Pagination page={1} pageSize={20} total={3} onChange={() => {}} />)
    expect(screen.getByText('3 registros')).toBeInTheDocument()
  })
  it('moves between pages and disables the ends', async () => {
    const user = userEvent.setup()
    const onChange = vi.fn()
    const { rerender } = render(<Pagination page={1} pageSize={20} total={45} onChange={onChange} />)
    expect(screen.getByText(/Página 1 de 3 · 45 registros/)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Anterior' })).toBeDisabled()
    await user.click(screen.getByRole('button', { name: 'Próxima' }))
    expect(onChange).toHaveBeenCalledWith(2)
    rerender(<Pagination page={3} pageSize={20} total={45} onChange={onChange} />)
    expect(screen.getByRole('button', { name: 'Próxima' })).toBeDisabled()
    await user.click(screen.getByRole('button', { name: 'Anterior' }))
    expect(onChange).toHaveBeenLastCalledWith(2)
  })
})

describe('States and StatusBadge', () => {
  it('announces loading, errors (with retry) and empty lists', async () => {
    const user = userEvent.setup()
    const retry = vi.fn()
    render(
      <>
        <Loading />
        <ErrorState error={new ApiError({ status: 0, code: 'network_error' })} onRetry={retry} />
        <Empty>Nada aqui</Empty>
      </>,
    )
    expect(screen.getByRole('status')).toHaveTextContent('Carregando…')
    expect(screen.getByRole('alert')).toHaveTextContent(/conexão/)
    await user.click(screen.getByRole('button', { name: 'Tentar novamente' }))
    expect(retry).toHaveBeenCalled()
    expect(screen.getByText('Nada aqui')).toBeInTheDocument()
  })
  it('omits the retry button when there is no handler', () => {
    render(<ErrorState error={new Error('x')} />)
    expect(screen.queryByRole('button')).not.toBeInTheDocument()
  })
  it('labels every appointment status in words (colour is never the only cue)', () => {
    render(
      <>
        <StatusBadge status="scheduled" />
        <StatusBadge status="completed" />
        <StatusBadge status="cancelled" />
        <StatusBadge status="no_show" />
      </>,
    )
    for (const label of ['Agendado', 'Concluído', 'Cancelado', 'Faltou']) expect(screen.getByText(label)).toBeInTheDocument()
  })
})

describe('DailyChart', () => {
  const days: Day[] = [
    { date: '2026-10-05', appointments: 2, completed: 1, revenue_cents: 5000 },
    { date: '2026-10-06', appointments: 0, completed: 0, revenue_cents: 0 },
    { date: '2026-10-07', appointments: 3, completed: 3, revenue_cents: 36000 },
  ]
  it('picks round axis tops that split into 4 equal steps', () => {
    expect(niceCeil(0)).toBe(1)
    expect(niceCeil(3)).toBe(5)
    expect(niceCeil(119000)).toBe(200000)
    expect(axisTop(0, 4)).toBe(4)
    expect(axisTop(3, 4)).toBe(4)
    expect(axisTop(7, 4)).toBe(8)
    expect(axisTop(11, 4)).toBe(20)
    expect(axisTop(36000, 400)).toBe(40000)
    expect(axisTop(0, 400)).toBe(400)
  })
  it('draws one bar per day with a text alternative and the same data as a table', () => {
    render(<DailyChart data={days} metric="revenue" />)
    const chart = screen.getByRole('img')
    expect(chart).toHaveAccessibleName(/total de R\$\s410,00 em 3 dias/)
    expect(chart.querySelectorAll('rect')).toHaveLength(3)
    expect(screen.getByText(/07\/10\/2026: R\$\s360,00/)).toBeInTheDocument() // bar tooltip
    const table = screen.getByRole('table', { name: 'Série diária' })
    expect(within(table).getAllByRole('row')).toHaveLength(4)
  })
  it('switches to counting appointments', () => {
    render(<DailyChart data={days} metric="appointments" />)
    expect(screen.getByRole('img')).toHaveAccessibleName(/total de 5 em 3 dias/)
    expect(screen.getByText('3 agendamento(s)', { exact: false })).toBeInTheDocument()
  })
  it('copes with an empty or all-zero series', () => {
    const { rerender } = render(<DailyChart data={[]} metric="revenue" />)
    expect(screen.getByRole('img').querySelectorAll('rect')).toHaveLength(0)
    rerender(<DailyChart data={[{ date: '2026-10-05', appointments: 0, completed: 0, revenue_cents: 0 }]} metric="appointments" />)
    expect(screen.getByRole('img').querySelectorAll('rect')).toHaveLength(1)
  })
})
