import { useState } from 'react'
import { errorMessage } from '../api/errors'
import { useAppointments, useSetStatus } from '../api/hooks'
import type { Appointment, AppointmentStatus } from '../api/types'
import { ConfirmDialog } from '../components/ConfirmDialog'
import { Field } from '../components/Field'
import { Pagination } from '../components/Pagination'
import { Empty, ErrorState, Loading } from '../components/States'
import { STATUS_LABEL, StatusBadge } from '../components/StatusBadge'
import { isDay, todayIn } from '../lib/datetime'
import { formatDate, formatDuration, formatMoney, formatTime } from '../lib/format'
import { AppointmentForm } from './AppointmentForm'

const PAGE_SIZE = 20

export function AppointmentsPage() {
  const [status, setStatus] = useState<'' | AppointmentStatus>('')
  const [from, setFrom] = useState(todayIn())
  const [to, setTo] = useState('')
  const [page, setPage] = useState(1)
  const rangeOk = (!from || isDay(from)) && (!to || isDay(to)) && !(from && to && from > to)
  const list = useAppointments({
    status: status || undefined,
    from: from && isDay(from) ? from : undefined,
    to: to && isDay(to) ? to : undefined,
    page,
    page_size: PAGE_SIZE,
  })
  const setStatusMutation = useSetStatus()
  const [creating, setCreating] = useState(false)
  const [cancelling, setCancelling] = useState<Appointment | null>(null)
  const [notice, setNotice] = useState<{ kind: 'ok' | 'error'; text: string } | null>(null)
  const now = Date.now()

  function change(a: Appointment, next: AppointmentStatus, after?: () => void) {
    setNotice(null)
    setStatusMutation.mutate(
      { id: a.id, status: next },
      {
        onSuccess: () => {
          setNotice({ kind: 'ok', text: `${a.customer_name}: ${STATUS_LABEL[next].toLowerCase()}.` })
          after?.()
        },
        onError: (e) => {
          setNotice({ kind: 'error', text: errorMessage(e) })
          after?.()
        },
      },
    )
  }

  const filter = (set: (v: string) => void) => (e: { target: { value: string } }) => {
    set(e.target.value)
    setPage(1)
  }

  return (
    <>
      <div className="page-head">
        <h1>Agenda</h1>
        <button type="button" className="btn btn-primary" onClick={() => setCreating(true)}>
          Novo agendamento
        </button>
      </div>
      <form className="filters" onSubmit={(e) => e.preventDefault()} aria-label="Filtros da agenda">
        <Field label="Situação">
          {(c) => (
            <select {...c} value={status} onChange={filter((v) => setStatus(v as typeof status))}>
              <option value="">Todas</option>
              {(Object.keys(STATUS_LABEL) as AppointmentStatus[]).map((s) => (
                <option key={s} value={s}>
                  {STATUS_LABEL[s]}
                </option>
              ))}
            </select>
          )}
        </Field>
        <Field label="De">{(c) => <input {...c} type="date" value={from} onChange={filter(setFrom)} />}</Field>
        <Field label="Até">{(c) => <input {...c} type="date" value={to} onChange={filter(setTo)} />}</Field>
        <button
          type="button"
          className="btn"
          onClick={() => {
            setStatus('')
            setFrom('')
            setTo('')
            setPage(1)
          }}
        >
          Limpar filtros
        </button>
      </form>
      {!rangeOk ? (
        <p className="banner banner-error" role="alert">
          A data inicial deve ser anterior ou igual à final.
        </p>
      ) : null}
      {notice ? (
        <p className={`banner ${notice.kind === 'ok' ? 'banner-ok' : 'banner-error'}`} role={notice.kind === 'ok' ? 'status' : 'alert'}>
          {notice.text}
        </p>
      ) : null}
      <section className="card" aria-label="Lista de agendamentos">
        {list.isPending ? (
          <Loading />
        ) : list.isError ? (
          <ErrorState error={list.error} onRetry={() => void list.refetch()} />
        ) : list.data.data.length === 0 ? (
          <Empty>Nenhum agendamento com esses filtros.</Empty>
        ) : (
          <>
            <div className="table-wrap">
              <table className="data stack">
                <thead>
                  <tr>
                    <th scope="col">Início</th>
                    <th scope="col">Cliente</th>
                    <th scope="col">Serviço</th>
                    <th scope="col" className="num">
                      Valor
                    </th>
                    <th scope="col">Situação</th>
                    <th scope="col">
                      <span className="sr-only">Ações</span>
                    </th>
                  </tr>
                </thead>
                <tbody>
                  {list.data.data.map((a) => {
                    const started = Date.parse(a.starts_at) <= now
                    return (
                      <tr key={a.id}>
                        <td data-label="Início">
                          <div>
                            <strong>{formatDate(a.starts_at)}</strong>
                            <div className="muted">
                              {formatTime(a.starts_at)}–{formatTime(a.ends_at)}
                            </div>
                          </div>
                        </td>
                        <td data-label="Cliente">{a.customer_name}</td>
                        <td data-label="Serviço">
                          <div>
                            {a.service_name} <span className="muted">· {formatDuration(a.duration_min)}</span>
                          </div>
                        </td>
                        <td data-label="Valor" className="num">
                          {formatMoney(a.price_cents)}
                        </td>
                        <td data-label="Situação">
                          <StatusBadge status={a.status} />
                        </td>
                        <td className="row-actions">
                          {a.status === 'scheduled' ? (
                            <div className="actions">
                              <button
                                type="button"
                                className="btn btn-small"
                                disabled={!started || setStatusMutation.isPending}
                                title={started ? undefined : 'Disponível depois do horário de início'}
                                onClick={() => change(a, 'completed')}
                              >
                                Concluir<span className="sr-only"> {a.customer_name}</span>
                              </button>
                              <button
                                type="button"
                                className="btn btn-small"
                                disabled={!started || setStatusMutation.isPending}
                                title={started ? undefined : 'Disponível depois do horário de início'}
                                onClick={() => change(a, 'no_show')}
                              >
                                Faltou<span className="sr-only"> {a.customer_name}</span>
                              </button>
                              <button type="button" className="btn btn-small btn-danger" onClick={() => setCancelling(a)}>
                                Cancelar<span className="sr-only"> {a.customer_name}</span>
                              </button>
                            </div>
                          ) : null}
                        </td>
                      </tr>
                    )
                  })}
                </tbody>
              </table>
            </div>
            <Pagination page={page} pageSize={PAGE_SIZE} total={list.data.total} onChange={setPage} />
          </>
        )}
      </section>

      {creating ? (
        <AppointmentForm
          onClose={() => setCreating(false)}
          onSaved={(text) => {
            setCreating(false)
            setNotice({ kind: 'ok', text })
          }}
        />
      ) : null}
      {cancelling ? (
        <ConfirmDialog
          title="Cancelar agendamento"
          message={`Cancelar o agendamento de ${cancelling.customer_name} (${cancelling.service_name}) em ${formatDate(cancelling.starts_at)} às ${formatTime(cancelling.starts_at)}? O horário volta a ficar livre.`}
          confirmLabel="Cancelar agendamento"
          busy={setStatusMutation.isPending}
          onCancel={() => setCancelling(null)}
          onConfirm={() => change(cancelling, 'cancelled', () => setCancelling(null))}
        />
      ) : null}
    </>
  )
}
