import type { Api } from '../api'
import { API_URL } from '../config'
import { ApiError } from '../types'

/*
 * Реальные запросы к сервису Domain. Эндпоинты — черновик, их описание для команды
 * будет в docs/API.md (Этап 3). Сессия — cookie, которую ставит Domain после входа.
 */

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  let response: Response
  try {
    response = await fetch(`${API_URL}${path}`, {
      method,
      credentials: 'include',
      headers: body === undefined ? undefined : { 'Content-Type': 'application/json' },
      body: body === undefined ? undefined : JSON.stringify(body),
    })
  } catch {
    throw new ApiError('network_error', 'Нет связи с сервером. Проверьте подключение.')
  }

  if (response.status === 204) return undefined as T

  const data: unknown = await response.json().catch(() => null)
  if (!response.ok) {
    const error = (data ?? {}) as { error?: string; message?: string }
    throw new ApiError(
      error.error ?? 'http_error',
      error.message ?? `Сервер ответил ошибкой ${response.status}.`,
      response.status,
    )
  }
  return data as T
}

export const httpApi: Api = {
  getSession: async () => {
    try {
      return await request('GET', '/api/session')
    } catch (error) {
      if (error instanceof ApiError && error.status === 401) return null
      throw error
    }
  },
  login: (credentials) => request('POST', '/api/session', credentials),
  register: (credentials) => request('POST', '/api/users', credentials),
  logout: () => request('DELETE', '/api/session'),

  getProfile: async () => {
    try {
      return await request('GET', '/api/company/profile')
    } catch (error) {
      if (error instanceof ApiError && error.status === 404) return null
      throw error
    }
  },
  saveProfile: (input) => request('PUT', '/api/company/profile', input),

  getSources: () => request('GET', '/api/sources'),
  addSource: (url) => request('POST', '/api/sources', { url }),
  setSourcePaused: (id, paused) =>
    request('PATCH', `/api/sources/${encodeURIComponent(id)}`, { paused }),
  deleteSource: (id) => request('DELETE', `/api/sources/${encodeURIComponent(id)}`),

  getMaterials: ({ from, to, limit }) =>
    request('GET', `/api/materials?${new URLSearchParams({ from, to, limit: String(limit) })}`),
  addMaterial: (url) => request('POST', '/api/materials', { url }),

  getChat: () => request('GET', '/api/chat'),
  sendMessage: (text) => request('POST', '/api/chat/messages', { text }),
}
