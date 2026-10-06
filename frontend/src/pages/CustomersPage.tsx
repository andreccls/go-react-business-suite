import { useState, type FormEvent } from 'react'
import { errorMessage, fieldErrorsFrom, isApiError } from '../api/errors'
import { useCreateCustomer, useCustomers, useDeleteCustomer, useUpdateCustomer } from '../api/hooks'
import type { Customer } from '../api/types'
import { useUser } from '../auth/AuthContext'
import { ConfirmDialog } from '../components/ConfirmDialog'
import { Field } from '../components/Field'
import { Modal } from '../components/Modal'
import { Pagination } from '../components/Pagination'
import { Empty, ErrorState, Loading } from '../components/States'
import { formatDate, formatPhone } from '../lib/format'
import { useDebounced } from '../lib/useDebounced'
import { validateCustomer, type FieldErrors } from '../lib/validation'

const PAGE_SIZE = 20

export function CustomersPage() {
  const isAdmin = useUser().role === 'admin'
  const [search, setSearch] = useState('')
  const [page, setPage] = useState(1)
  const q = useDebounced(search.trim())
  const list = useCustomers({ q: q || undefined, page, page_size: PAGE_SIZE })
  const remove = useDeleteCustomer()
  const [editing, setEditing] = useState<Customer | 'new' | null>(null)
  const [deleting, setDeleting] = useState<Customer | null>(null)
  const [notice, setNotice] = useState<string | null>(null)

  return (
    <>
      <div className="page-head">
        <h1>Clientes</h1>
        <button type="button" className="btn btn-primary" onClick={() => setEditing('new')}>
          Novo cliente
        </button>
      </div>
      <form className="filters" onSubmit={(e) => e.preventDefault()} role="search" aria-label="Buscar clientes">
        <Field label="Buscar por nome ou e-mail">
          {(c) => (
            <input
              {...c}
              type="search"
              value={search}
              onChange={(e) => {
                setSearch(e.target.value)
                setPage(1)
              }}
            />
          )}
        </Field>
      </form>
      {notice ? (
        <p className="banner banner-ok" role="status">
          {notice}
        </p>
      ) : null}
      <section className="card" aria-label="Lista de clientes">
        {list.isPending ? (
          <Loading />
        ) : list.isError ? (
          <ErrorState error={list.error} onRetry={() => void list.refetch()} />
        ) : list.data.data.length === 0 ? (
          <Empty>{q ? 'Nenhum cliente encontrado para essa busca.' : 'Nenhum cliente cadastrado ainda.'}</Empty>
        ) : (
          <>
            <div className="table-wrap">
              <table className="data stack">
                <thead>
                  <tr>
                    <th scope="col">Cliente</th>
                    <th scope="col">Telefone</th>
                    <th scope="col">Cadastro</th>
                    <th scope="col">
                      <span className="sr-only">Ações</span>
                    </th>
                  </tr>
                </thead>
                <tbody>
                  {list.data.data.map((c) => (
                    <tr key={c.id}>
                      <td data-label="Cliente">
                        <div>
                          <strong>{c.name}</strong>
                          <div className="muted">{c.email}</div>
                        </div>
                      </td>
                      <td data-label="Telefone">{c.phone ? formatPhone(c.phone) : '—'}</td>
                      <td data-label="Cadastro">{formatDate(c.created_at)}</td>
                      <td className="row-actions">
                        <div className="actions">
                          <button type="button" className="btn btn-small" onClick={() => setEditing(c)}>
                            Editar<span className="sr-only"> {c.name}</span>
                          </button>
                          {isAdmin ? (
                            <button
                              type="button"
                              className="btn btn-small btn-danger"
                              onClick={() => {
                                remove.reset()
                                setDeleting(c)
                              }}
                            >
                              Excluir<span className="sr-only"> {c.name}</span>
                            </button>
                          ) : null}
                        </div>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
            <Pagination page={page} pageSize={PAGE_SIZE} total={list.data.total} onChange={setPage} />
          </>
        )}
      </section>

      {editing ? (
        <CustomerForm
          customer={editing === 'new' ? null : editing}
          onClose={() => setEditing(null)}
          onSaved={(text) => {
            setEditing(null)
            setNotice(text)
          }}
        />
      ) : null}
      {deleting ? (
        <ConfirmDialog
          title="Excluir cliente"
          message={`Excluir "${deleting.name}" de vez? Clientes com agendamentos não podem ser excluídos.`}
          confirmLabel="Excluir"
          busy={remove.isPending}
          error={remove.error}
          onCancel={() => setDeleting(null)}
          onConfirm={() =>
            remove.mutate(deleting.id, {
              onSuccess: () => {
                setNotice(`Cliente "${deleting.name}" excluído.`)
                setDeleting(null)
              },
            })
          }
        />
      ) : null}
    </>
  )
}

function CustomerForm({ customer, onClose, onSaved }: { customer: Customer | null; onClose: () => void; onSaved: (msg: string) => void }) {
  const create = useCreateCustomer()
  const update = useUpdateCustomer()
  const [v, setV] = useState({
    name: customer?.name ?? '',
    email: customer?.email ?? '',
    phone: customer?.phone ? formatPhone(customer.phone) : '',
    notes: customer?.notes ?? '',
  })
  const [errors, setErrors] = useState<FieldErrors>({})
  const [banner, setBanner] = useState<string | null>(null)
  const busy = create.isPending || update.isPending
  const set = (k: keyof typeof v) => (e: { target: { value: string } }) => setV((p) => ({ ...p, [k]: e.target.value }))

  function submit(e: FormEvent) {
    e.preventDefault()
    setBanner(null)
    const fe = validateCustomer(v)
    setErrors(fe)
    if (Object.keys(fe).length > 0) return
    const body = { name: v.name.trim(), email: v.email.trim(), phone: v.phone.trim(), notes: v.notes.trim() }
    const opts = {
      onSuccess: () => onSaved(customer ? `Cliente "${body.name}" atualizado.` : `Cliente "${body.name}" cadastrado.`),
      onError: (err: unknown) => {
        const server = fieldErrorsFrom(err)
        if (Object.keys(server).length > 0) setErrors(server)
        else if (isApiError(err, 'email_taken')) setErrors({ email: errorMessage(err) })
        else setBanner(errorMessage(err))
      },
    }
    if (customer) update.mutate({ id: customer.id, patch: body }, opts)
    else create.mutate(body, opts)
  }

  return (
    <Modal title={customer ? 'Editar cliente' : 'Novo cliente'} onClose={onClose}>
      <form onSubmit={submit} noValidate className="form-grid">
        {banner ? (
          <p className="banner banner-error" role="alert">
            {banner}
          </p>
        ) : null}
        <Field label="Nome" error={errors.name} required>
          {(c) => <input {...c} value={v.name} onChange={set('name')} maxLength={120} autoComplete="off" />}
        </Field>
        <Field label="E-mail" error={errors.email} required>
          {(c) => <input {...c} type="email" value={v.email} onChange={set('email')} autoComplete="off" />}
        </Field>
        <Field label="Telefone" error={errors.phone} hint="Opcional. Com DDD, ex.: (31) 99999-0000.">
          {(c) => <input {...c} type="tel" value={v.phone} onChange={set('phone')} autoComplete="off" />}
        </Field>
        <Field label="Observações" error={errors.notes}>
          {(c) => <textarea {...c} value={v.notes} onChange={set('notes')} maxLength={1000} />}
        </Field>
        <div className="modal-actions">
          <button type="button" className="btn" onClick={onClose}>
            Cancelar
          </button>
          <button type="submit" className="btn btn-primary" disabled={busy}>
            {busy ? 'Salvando…' : 'Salvar'}
          </button>
        </div>
      </form>
    </Modal>
  )
}
