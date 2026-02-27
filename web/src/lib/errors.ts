import { Code, ConnectError } from '@connectrpc/connect'

type ApiErrorPayload = {
  code?: string
  message?: string
}

export class ApiError extends Error {
  status: number
  code: string

  constructor(message: string, status: number, code = '') {
    super(message)
    this.status = status
    this.code = code
  }
}

export function parseApiErrorPayload(payload: unknown): ApiErrorPayload | null {
  if (payload == null || typeof payload !== 'object') {
    return null
  }

  const value = payload as { code?: unknown; message?: unknown }
  return {
    code: typeof value.code === 'string' ? value.code : undefined,
    message: typeof value.message === 'string' ? value.message : undefined,
  }
}

export function messageFromError(error: unknown): string {
  if (error instanceof ConnectError) {
    if (error.code === Code.Unavailable) {
      return 'service unavailable'
    }
    return error.rawMessage !== '' ? error.rawMessage : 'request failed'
  }
  if (error instanceof ApiError) {
    return error.message
  }
  if (error instanceof Error && error.message !== '') {
    return error.message
  }
  return 'request failed'
}
