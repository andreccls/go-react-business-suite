import { Link } from 'react-router'

export function NotFoundPage() {
  return (
    <div className="card">
      <h1>Página não encontrada</h1>
      <p className="muted">
        O endereço não existe. <Link to="/">Voltar ao painel</Link>
      </p>
    </div>
  )
}
