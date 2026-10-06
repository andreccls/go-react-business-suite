import type { ErrorCode, FieldError } from './types'

/** Every failed call surfaces as one of these: HTTP problems (RFC 9457) and network failures alike. */
export class ApiError extends Error {
  readonly status: number
  readonly code: ErrorCode | 'network_error' | 'unexpected_response'
  readonly fields: FieldError[]
  readonly requestId?: string
  readonly retryAfter?: number

  constructor(init: {
    status: number
    code: ApiError['code']
    detail?: string
    fields?: FieldError[]
    requestId?: string
    retryAfter?: number
  }) {
    super(init.detail ?? init.code)
    this.name = 'ApiError'
    this.status = init.status
    this.code = init.code
    this.fields = init.fields ?? []
    this.requestId = init.requestId
    this.retryAfter = init.retryAfter
  }
}

const MESSAGES: Partial<Record<ApiError['code'], string>> = {
  invalid_credentials: 'E-mail ou senha incorretos.',
  invalid_token: 'Sua sessão expirou. Entre novamente.',
  missing_token: 'Sua sessão expirou. Entre novamente.',
  forbidden: 'Você não tem permissão para fazer isso.',
  email_taken: 'Já existe um cadastro com esse e-mail.',
  slot_unavailable: 'Esse horário conflita com outro agendamento. Escolha outro horário.',
  not_started: 'Só é possível concluir ou marcar falta depois do horário de início.',
  invalid_transition: 'Esse agendamento já foi encerrado e não pode mudar de status.',
  service_in_use: 'Este serviço tem agendamentos e não pode ser excluído. Desative-o em vez disso.',
  customer_in_use: 'Este cliente tem agendamentos e não pode ser excluído.',
  service_inactive: 'Este serviço está inativo e não pode ser agendado.',
  unknown_reference: 'O cliente ou o serviço foi removido enquanto você preenchia. Recarregue a página.',
  service_not_found: 'Serviço não encontrado. Ele pode ter sido removido.',
  customer_not_found: 'Cliente não encontrado. Ele pode ter sido removido.',
  appointment_not_found: 'Agendamento não encontrado.',
  body_too_large: 'O conteúdo enviado é grande demais.',
  timeout: 'O servidor demorou demais para responder. Tente novamente.',
  not_ready: 'O servidor está indisponível no momento. Tente novamente em instantes.',
  internal_error: 'Erro inesperado no servidor. Tente novamente.',
  network_error: 'Não foi possível falar com o servidor. Verifique sua conexão.',
  unexpected_response: 'O servidor respondeu de um jeito inesperado. Tente novamente.',
}

/** A message for people, in Portuguese; falls back to the API's own `detail`. */
export function errorMessage(err: unknown): string {
  if (err instanceof ApiError) {
    if (err.code === 'rate_limited') {
      return err.retryAfter
        ? `Muitas tentativas. Tente novamente em ${err.retryAfter} s.`
        : 'Muitas tentativas. Aguarde um instante e tente novamente.'
    }
    return MESSAGES[err.code] ?? err.message
  }
  return 'Algo deu errado. Tente novamente.'
}

/** 422 `errors[{field,message}]` → a field → message map (the API's text is kept as is). */
export function fieldErrorsFrom(err: unknown): Record<string, string> {
  const out: Record<string, string> = {}
  if (err instanceof ApiError) for (const f of err.fields) out[f.field] = f.message
  return out
}

export function isApiError(err: unknown, code?: ApiError['code']): err is ApiError {
  return err instanceof ApiError && (code === undefined || err.code === code)
}
