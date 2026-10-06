import { useState, type FormEvent } from 'react'
import { errorMessage, fieldErrorsFrom } from '../api/errors'
import { useCreateService, useDeleteService, useServices, useUpdateService } from '../api/hooks'
import type { Service } from '../api/types'
import { useUser } from '../auth/AuthContext'
import { ConfirmDialog } from '../components/ConfirmDialog'
import { Field } from '../components/Field'
import { Modal } from '../components/Modal'
import { Pagination } from '../components/Pagination'
import { Empty, ErrorState, Loading } from '../components/States'
import { centsToInput, formatDuration, formatMoney, parseMoneyToCents } from '../lib/format'
import { useDebounced } from '../lib/useDebounced'
import { validateService, type FieldErrors } from '../lib/validation'

const PAGE_SIZE = 20

export function ServicesPage() {
  const isAdmin = useUser().role === 'admin'
  const [search, setSearch] = useState('')
  const [active, setActive] = useState<'' | 'true' | 'false'>('')
  const [page, setPage] = useState(1)
  const q = useDebounced(search.trim())
  const list = useServices({ q: q || undefined, active: active === '' ? undefined : active === 'true', page, page_size: PAGE_SIZE })
  const update = useUpdateService()
  const remove = useDeleteService()
  const [editing, setEditing] = useState<Service | 'new' | null>(null)
  const [deleting, setDeleting] = useState<Service | null>(null)
  const [notice, setNotice] = useState<{ kind: 'ok' | 'error'; text: string } | null>(null)

  function toggleActive(s: Service) {
    setNotice(null)
    update.mutate(
      { id: s.id, patch: { active: !s.active } },
      {
        onSuccess: () => setNotice({ kind: 'ok', text: `Serviço "${s.name}" ${s.active ? 'desativado' : 'ativado'}.` }),
        onError: (e) => setNotice({ kind: 'error', text: errorMessage(e) }),
      },
    )
  }

  return (
    <>
      <div className="page-head">
        <h1>Serviços</h1>
        <button type="button" className="btn btn-primary" onClick={() => setEditing('new')}>
          Novo serviço
        </button>
      </div>
      <form className="filters" onSubmit={(e) => e.preventDefault()} role="search" aria-label="Filtrar serviços">
        <Field label="Buscar por nome">
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
        <Field label="Situação">
          {(c) => (
            <select
              {...c}
              value={active}
              onChange={(e) => {
                setActive(e.target.value as typeof active)
                setPage(1)
              }}
            >
              <option value="">Todos</option>
              <option value="true">Ativos</option>
              <option value="false">Inativos</option>
            </select>
          )}
        </Field>
      </form>
      {notice ? (
        <p className={`banner ${notice.kind === 'ok' ? 'banner-ok' : 'banner-error'}`} role={notice.kind === 'ok' ? 'status' : 'alert'}>
          {notice.text}
        </p>
      ) : null}
      <section className="card" aria-label="Lista de serviços">
        {list.isPending ? (
          <Loading />
        ) : list.isError ? (
          <ErrorState error={list.error} onRetry={() => void list.refetch()} />
        ) : list.data.data.length === 0 ? (
          <Empty>{q || active ? 'Nenhum serviço encontrado com esses filtros.' : 'Nenhum serviço cadastrado ainda.'}</Empty>
        ) : (
          <>
            <div className="table-wrap">
              <table className="data stack">
                <thead>
                  <tr>
                    <th scope="col">Serviço</th>
                    <th scope="col">Duração</th>
                    <th scope="col" className="num">
                      Preço
                    </th>
                    <th scope="col">Situação</th>
                    <th scope="col">
                      <span className="sr-only">Ações</span>
                    </th>
                  </tr>
                </thead>
                <tbody>
                  {list.data.data.map((s) => (
                    <tr key={s.id}>
                      <td data-label="Serviço">
                        <strong>{s.name}</strong>
                        {s.description ? <div className="muted">{s.description}</div> : null}
                      </td>
                      <td data-label="Duração">{formatDuration(s.duration_min)}</td>
                      <td data-label="Preço" className="num">
                        {formatMoney(s.price_cents)}
                      </td>
                      <td data-label="Situação">
                        <span className={`badge ${s.active ? 'badge-active' : 'badge-inactive'}`}>{s.active ? 'Ativo' : 'Inativo'}</span>
                      </td>
                      <td className="row-actions">
                        <div className="actions">
                          <button type="button" className="btn btn-small" onClick={() => setEditing(s)} aria-label={`Editar ${s.name}`}>
                            Editar
                          </button>
                          <button type="button" className="btn btn-small" onClick={() => toggleActive(s)} disabled={update.isPending}>
                            {s.active ? 'Desativar' : 'Ativar'}
                            <span className="sr-only"> {s.name}</span>
                          </button>
                          {isAdmin ? (
                            <button
                              type="button"
                              className="btn btn-small btn-danger"
                              onClick={() => {
                                remove.reset()
                                setDeleting(s)
                              }}
                            >
                              Excluir<span className="sr-only"> {s.name}</span>
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
        <ServiceForm
          service={editing === 'new' ? null : editing}
          onClose={() => setEditing(null)}
          onSaved={(text) => {
            setEditing(null)
            setNotice({ kind: 'ok', text })
          }}
        />
      ) : null}
      {deleting ? (
        <ConfirmDialog
          title="Excluir serviço"
          message={`Excluir "${deleting.name}" de vez? Se houver agendamentos com ele, a exclusão será recusada — nesse caso, desative-o.`}
          confirmLabel="Excluir"
          busy={remove.isPending}
          error={remove.error}
          onCancel={() => setDeleting(null)}
          onConfirm={() =>
            remove.mutate(deleting.id, {
              onSuccess: () => {
                setNotice({ kind: 'ok', text: `Serviço "${deleting.name}" excluído.` })
                setDeleting(null)
              },
            })
          }
        />
      ) : null}
    </>
  )
}

function ServiceForm({ service, onClose, onSaved }: { service: Service | null; onClose: () => void; onSaved: (msg: string) => void }) {
  const create = useCreateService()
  const update = useUpdateService()
  const [v, setV] = useState({
    name: service?.name ?? '',
    description: service?.description ?? '',
    duration_min: String(service?.duration_min ?? 60),
    price_cents: service ? centsToInput(service.price_cents) : '',
    active: service?.active ?? true,
  })
  const [errors, setErrors] = useState<FieldErrors>({})
  const [banner, setBanner] = useState<string | null>(null)
  const busy = create.isPending || update.isPending
  const set = (k: keyof typeof v) => (e: { target: { value: string } }) => setV((p) => ({ ...p, [k]: e.target.value }))

  function submit(e: FormEvent) {
    e.preventDefault()
    setBanner(null)
    const fe = validateService(v)
    setErrors(fe)
    if (Object.keys(fe).length > 0) return
    const body = {
      name: v.name.trim(),
      description: v.description.trim(),
      duration_min: Number(v.duration_min),
      price_cents: parseMoneyToCents(v.price_cents) ?? 0,
      active: v.active,
    }
    const opts = {
      onSuccess: () => onSaved(service ? `Serviço "${body.name}" atualizado.` : `Serviço "${body.name}" criado.`),
      onError: (err: unknown) => {
        const server = fieldErrorsFrom(err)
        if (Object.keys(server).length > 0) setErrors(server)
        else setBanner(errorMessage(err))
      },
    }
    if (service) update.mutate({ id: service.id, patch: body }, opts)
    else create.mutate(body, opts)
  }

  return (
    <Modal title={service ? 'Editar serviço' : 'Novo serviço'} onClose={onClose}>
      <form onSubmit={submit} noValidate className="form-grid">
        {banner ? (
          <p className="banner banner-error" role="alert">
            {banner}
          </p>
        ) : null}
        <Field label="Nome" error={errors.name} required>
          {(c) => <input {...c} value={v.name} onChange={set('name')} maxLength={120} />}
        </Field>
        <Field label="Descrição" error={errors.description}>
          {(c) => <textarea {...c} value={v.description} onChange={set('description')} maxLength={500} />}
        </Field>
        <div className="form-row">
          <Field label="Duração (minutos)" error={errors.duration_min} hint="De 5 a 480." required>
            {(c) => <input {...c} type="number" inputMode="numeric" min={5} max={480} value={v.duration_min} onChange={set('duration_min')} />}
          </Field>
          <Field label="Preço (R$)" error={errors.price_cents} hint="Ex.: 120,00" required>
            {(c) => <input {...c} inputMode="decimal" value={v.price_cents} onChange={set('price_cents')} />}
          </Field>
        </div>
        <label className="check">
          <input type="checkbox" checked={v.active} onChange={(e) => setV((p) => ({ ...p, active: e.target.checked }))} />
          Ativo (pode ser agendado)
        </label>
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
