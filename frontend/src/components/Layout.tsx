import { NavLink, Outlet } from 'react-router'
import { useAuth, useUser } from '../auth/AuthContext'

const ROLE_LABEL = { admin: 'Administrador', staff: 'Equipe' } as const

export function Layout() {
  const user = useUser()
  const { logout } = useAuth()
  return (
    <>
      <a href="#conteudo" className="skip-link">
        Ir para o conteúdo
      </a>
      <header className="app-header">
        <div className="app-header-inner">
          <span className="brand">Studio Suite</span>
          <nav className="main-nav" aria-label="Principal">
            <NavLink to="/" end>
              Painel
            </NavLink>
            <NavLink to="/agenda">Agenda</NavLink>
            <NavLink to="/servicos">Serviços</NavLink>
            <NavLink to="/clientes">Clientes</NavLink>
            {user.role === 'admin' ? <NavLink to="/usuarios">Usuários</NavLink> : null}
          </nav>
          <div className="user-box">
            <span>
              {user.email} · <span className={`badge badge-${user.role}`}>{ROLE_LABEL[user.role]}</span>
            </span>
            <button type="button" className="btn btn-small" onClick={() => void logout()}>
              Sair
            </button>
          </div>
        </div>
      </header>
      <main id="conteudo" className="page" tabIndex={-1}>
        <Outlet />
      </main>
    </>
  )
}
