import { useState, type FormEvent } from 'react'
import { errorMessage, fieldErrorsFrom, isApiError } from '../api/errors'
import { useCreateUser } from '../api/hooks'
import type { Role, User } from '../api/types'
import { Field } from '../components/Field'
import { validateUser, type FieldErrors } from '../lib/validation'

const ROLE_LABEL: Record<Role, string> = { admin: 'Administrador', staff: 'Equipe (não remove dados)' }

/** The contract has no "list users" endpoint, so this screen only creates and shows what THIS session created. */
export function UsersPage() {
  const create = useCreateUser()
  const [v, setV] = useState({ email: '', password: '', role: 'staff' })
  const [errors, setErrors] = useState<FieldErrors>({})
  const [banner, setBanner] = useState<string | null>(null)
  const [created, setCreated] = useState<User[]>([])

  function submit(e: FormEvent) {
    e.preventDefault()
    setBanner(null)
    const fe = validateUser(v)
    setErrors(fe)
    if (Object.keys(fe).length > 0) return
    create.mutate(
      { email: v.email.trim(), password: v.password, role: v.role as Role },
      {
        onSuccess: (u) => {
          setCreated((c) => [u, ...c])
          setV({ email: '', password: '', role: 'staff' })
        },
        onError: (err) => {
          const server = fieldErrorsFrom(err)
          if (Object.keys(server).length > 0) setErrors(server)
          else if (isApiError(err, 'email_taken')) setErrors({ email: errorMessage(err) })
          else setBanner(errorMessage(err))
        },
      },
    )
  }

  return (
    <>
      <h1>Usuários</h1>
      <div className="grid-2">
        <section className="card" aria-labelledby="new-user">
          <h2 id="new-user">Novo usuário</h2>
          <form onSubmit={submit} noValidate className="form-grid">
            {banner ? (
              <p className="banner banner-error" role="alert">
                {banner}
              </p>
            ) : null}
            <Field label="E-mail" error={errors.email} required>
              {(c) => <input {...c} type="email" autoComplete="off" value={v.email} onChange={(e) => setV({ ...v, email: e.target.value })} />}
            </Field>
            <Field label="Senha inicial" error={errors.password} hint="De 8 a 72 caracteres." required>
              {(c) => (
                <input {...c} type="password" autoComplete="new-password" value={v.password} onChange={(e) => setV({ ...v, password: e.target.value })} />
              )}
            </Field>
            <Field label="Papel" error={errors.role} required>
              {(c) => (
                <select {...c} value={v.role} onChange={(e) => setV({ ...v, role: e.target.value })}>
                  <option value="staff">{ROLE_LABEL.staff}</option>
                  <option value="admin">{ROLE_LABEL.admin}</option>
                </select>
              )}
            </Field>
            <button type="submit" className="btn btn-primary" disabled={create.isPending}>
              {create.isPending ? 'Criando…' : 'Criar usuário'}
            </button>
          </form>
        </section>
        <section className="card" aria-labelledby="created">
          <h2 id="created">Criados nesta sessão</h2>
          {created.length === 0 ? (
            <p className="muted">
              Nenhum ainda. A API não oferece listagem de usuários, então só aparecem aqui os que você criar agora.
            </p>
          ) : (
            <ul>
              {created.map((u) => (
                <li key={u.id}>
                  {u.email} · <span className={`badge badge-${u.role}`}>{u.role === 'admin' ? 'Administrador' : 'Equipe'}</span>
                </li>
              ))}
            </ul>
          )}
        </section>
      </div>
    </>
  )
}
