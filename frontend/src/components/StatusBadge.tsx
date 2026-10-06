import type { AppointmentStatus } from '../api/types'

export const STATUS_LABEL: Record<AppointmentStatus, string> = {
  scheduled: 'Agendado',
  completed: 'Concluído',
  cancelled: 'Cancelado',
  no_show: 'Faltou',
}

export function StatusBadge({ status }: { status: AppointmentStatus }) {
  return <span className={`badge badge-${status}`}>{STATUS_LABEL[status]}</span>
}
