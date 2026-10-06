import type { ReactNode } from 'react'
import { errorMessage } from '../api/errors'

export function Loading({ label = 'Carregando…' }: { label?: string }) {
  return (
    <div className="state" role="status">
      <span className="spinner" aria-hidden="true" />
      <span>{label}</span>
    </div>
  )
}

export function ErrorState({ error, onRetry }: { error: unknown; onRetry?: () => void }) {
  return (
    <div className="state error" role="alert">
      <span>{errorMessage(error)}</span>
      {onRetry ? (
        <button type="button" className="btn" onClick={onRetry}>
          Tentar novamente
        </button>
      ) : null}
    </div>
  )
}

export function Empty({ children }: { children: ReactNode }) {
  return <div className="state">{children}</div>
}
