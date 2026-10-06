import { errorMessage } from '../api/errors'
import { Modal } from './Modal'

interface Props {
  title: string
  message: string
  confirmLabel: string
  busy?: boolean
  error?: unknown
  onConfirm: () => void
  onCancel: () => void
}

export function ConfirmDialog({ title, message, confirmLabel, busy, error, onConfirm, onCancel }: Props) {
  return (
    <Modal title={title} onClose={onCancel}>
      <p>{message}</p>
      {error ? (
        <p className="banner banner-error mt" role="alert">
          {errorMessage(error)}
        </p>
      ) : null}
      <div className="modal-actions">
        <button type="button" className="btn" onClick={onCancel}>
          Voltar
        </button>
        <button type="button" className="btn btn-danger" onClick={onConfirm} disabled={busy}>
          {busy ? 'Aguarde…' : confirmLabel}
        </button>
      </div>
    </Modal>
  )
}
