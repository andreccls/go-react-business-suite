import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from './index'
import type {
  AppointmentInput,
  AppointmentQuery,
  AppointmentStatus,
  CustomerInput,
  CustomerPatch,
  CustomerQuery,
  NewUserRequest,
  PeriodQuery,
  ServiceInput,
  ServicePatch,
  ServiceQuery,
} from './types'

// Server state lives in TanStack Query (docs/adr/0006-server-state-tanstack-query.md): every
// screen reads through these hooks; writes invalidate the keys they can affect.

const list = { placeholderData: keepPreviousData }

export const useServices = (q: ServiceQuery) => useQuery({ queryKey: ['services', q], queryFn: () => api.listServices(q), ...list })
export const useCustomers = (q: CustomerQuery) => useQuery({ queryKey: ['customers', q], queryFn: () => api.listCustomers(q), ...list })
export const useAppointments = (q: AppointmentQuery) =>
  useQuery({ queryKey: ['appointments', q], queryFn: () => api.listAppointments(q), ...list })

export const useSummary = (p: PeriodQuery) => useQuery({ queryKey: ['dashboard', 'summary', p], queryFn: () => api.summary(p) })
export const useDaily = (p: PeriodQuery) => useQuery({ queryKey: ['dashboard', 'daily', p], queryFn: () => api.daily(p) })
export const useTopServices = (p: PeriodQuery) =>
  useQuery({ queryKey: ['dashboard', 'top', p], queryFn: () => api.topServices({ ...p, limit: 5 }) })
export const useUpcoming = () => useQuery({ queryKey: ['dashboard', 'upcoming'], queryFn: () => api.upcoming(5) })

function useInvalidate() {
  const qc = useQueryClient()
  return (...roots: string[]) => Promise.all(roots.map((r) => qc.invalidateQueries({ queryKey: [r] })))
}

export function useCreateService() {
  const inv = useInvalidate()
  return useMutation({ mutationFn: (i: ServiceInput) => api.createService(i), onSuccess: () => inv('services') })
}
export function useUpdateService() {
  const inv = useInvalidate()
  return useMutation({
    mutationFn: ({ id, patch }: { id: string; patch: ServicePatch }) => api.updateService(id, patch),
    onSuccess: () => inv('services'),
  })
}
export function useDeleteService() {
  const inv = useInvalidate()
  return useMutation({ mutationFn: (id: string) => api.deleteService(id), onSuccess: () => inv('services') })
}

export function useCreateCustomer() {
  const inv = useInvalidate()
  return useMutation({ mutationFn: (i: CustomerInput) => api.createCustomer(i), onSuccess: () => inv('customers', 'dashboard') })
}
export function useUpdateCustomer() {
  const inv = useInvalidate()
  return useMutation({
    mutationFn: ({ id, patch }: { id: string; patch: CustomerPatch }) => api.updateCustomer(id, patch),
    // appointments embed the customer's current name
    onSuccess: () => inv('customers', 'appointments'),
  })
}
export function useDeleteCustomer() {
  const inv = useInvalidate()
  return useMutation({ mutationFn: (id: string) => api.deleteCustomer(id), onSuccess: () => inv('customers', 'dashboard') })
}

export function useCreateAppointment() {
  const inv = useInvalidate()
  return useMutation({ mutationFn: (i: AppointmentInput) => api.createAppointment(i), onSuccess: () => inv('appointments', 'dashboard') })
}
export function useSetStatus() {
  const inv = useInvalidate()
  return useMutation({
    mutationFn: ({ id, status }: { id: string; status: AppointmentStatus }) => api.setAppointmentStatus(id, status),
    onSettled: () => inv('appointments', 'dashboard'), // also after a 409: the list may be stale
  })
}

export const useCreateUser = () => useMutation({ mutationFn: (i: NewUserRequest) => api.createUser(i) })
