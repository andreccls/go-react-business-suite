import { useState, type FormEvent } from 'react'
import { errorMessage, fieldErrorsFrom } from '../api/errors'
import { useCreateAppointment, useCustomers, useServices } from '../api/hooks'
import { Field } from '../components/Field'
import { Modal } from '../components/Modal'
import { todayIn, zonedToInstant } from '../lib/datetime'
import { formatDuration, formatMoney, formatTime } from '../lib/format'
import { validateAppointment, type FieldErrors } from '../lib/validation'

/** Book an appointment: customer + active service + day/time in the business time zone. */
export function AppointmentForm({ onClose, onSaved }: { onClose: () => void; onSaved: (msg: string) => void }) {
  const customers = useCustomers({ page_size: 100 })
  const services = useServices({ active: true, page_size: 100 })
  const create = useCreateAppointment()
  const [v, setV] = useState({ customer_id: '', service_id: '', date: todayIn(), time: '09:00', notes: '' })
  const [errors, setErrors] = useState<FieldErrors>({})
  const [banner, setBanner] = useState<string | null>(null)
  const set = (k: keyof typeof v) => (e: { target: { value: string } }) => setV((p) => ({ ...p, [k]: e.target.value }))

  const service = services.data?.data.find((s) => s.id === v.service_id)
  const startsAt = zonedToInstant(v.date, v.time)
  const endsAt = service && startsAt ? new Date(Date.parse(startsAt) + service.duration_min * 60_000).toISOString() : null
  const loadFailed = customers.isError || services.isError

  function submit(e: FormEvent) {
    e.preventDefault()
    setBanner(null)
    const fe = validateAppointment(v)
    setErrors(fe)
    if (Object.keys(fe).length > 0 || !startsAt) return
    create.mutate(
      { customer_id: v.customer_id, service_id: v.service_id, starts_at: startsAt, notes: v.notes.trim() || undefined },
      {
        onSuccess: (a) => onSaved(`Agendamento de ${a.customer_name} (${a.service_name}) criado.`),
        onError: (err) => {
          const server = fieldErrorsFrom(err)
          if ('starts_at' in server) server.time = server.starts_at ?? ''
          if (Object.keys(server).length > 0) setErrors(server)
          else setBanner(errorMessage(err)) // includes the 409 slot_unavailable conflict
        },
      },
    )
  }

  return (
    <Modal title="Novo agendamento" onClose={onClose}>
      <form onSubmit={submit} noValidate className="form-grid">
        {banner ? (
          <p className="banner banner-error" role="alert">
            {banner}
          </p>
        ) : null}
        {loadFailed ? (
          <p className="banner banner-error" role="alert">
            Não foi possível carregar clientes e serviços. Feche e tente novamente.
          </p>
        ) : null}
        <Field label="Cliente" error={errors.customer_id} required>
          {(c) => (
            <select {...c} value={v.customer_id} onChange={set('customer_id')}>
              <option value="">{customers.isPending ? 'Carregando…' : 'Selecione…'}</option>
              {customers.data?.data.map((x) => (
                <option key={x.id} value={x.id}>
                  {x.name}
                </option>
              ))}
            </select>
          )}
        </Field>
        <Field label="Serviço" error={errors.service_id} hint={service ? `${formatDuration(service.duration_min)} · ${formatMoney(service.price_cents)}` : 'Só serviços ativos.'} required>
          {(c) => (
            <select {...c} value={v.service_id} onChange={set('service_id')}>
              <option value="">{services.isPending ? 'Carregando…' : 'Selecione…'}</option>
              {services.data?.data.map((x) => (
                <option key={x.id} value={x.id}>
                  {x.name}
                </option>
              ))}
            </select>
          )}
        </Field>
        <div className="form-row">
          <Field label="Data" error={errors.date} required>
            {(c) => <input {...c} type="date" value={v.date} onChange={set('date')} />}
          </Field>
          <Field
            label="Horário"
            error={errors.time}
            hint={endsAt ? `Termina às ${formatTime(endsAt)}` : 'No fuso do estúdio.'}
            required
          >
            {(c) => <input {...c} type="time" step={300} value={v.time} onChange={set('time')} />}
          </Field>
        </div>
        <Field label="Observações" error={errors.notes}>
          {(c) => <textarea {...c} value={v.notes} onChange={set('notes')} maxLength={500} />}
        </Field>
        <div className="modal-actions">
          <button type="button" className="btn" onClick={onClose}>
            Cancelar
          </button>
          <button type="submit" className="btn btn-primary" disabled={create.isPending}>
            {create.isPending ? 'Agendando…' : 'Agendar'}
          </button>
        </div>
      </form>
    </Modal>
  )
}
