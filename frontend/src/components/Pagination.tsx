interface Props {
  page: number
  pageSize: number
  total: number
  onChange: (page: number) => void
}

export function Pagination({ page, pageSize, total, onChange }: Props) {
  const pages = Math.max(1, Math.ceil(total / pageSize))
  if (total <= pageSize) return <p className="muted pagination">{total} {total === 1 ? 'registro' : 'registros'}</p>
  return (
    <nav className="pagination" aria-label="Paginação">
      <span className="muted">
        Página {page} de {pages} · {total} registros
      </span>
      <span className="actions">
        <button type="button" className="btn btn-small" disabled={page <= 1} onClick={() => onChange(page - 1)}>
          Anterior
        </button>
        <button type="button" className="btn btn-small" disabled={page >= pages} onClick={() => onChange(page + 1)}>
          Próxima
        </button>
      </span>
    </nav>
  )
}
