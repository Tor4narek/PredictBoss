import type { Api } from '../api'
import { API_URL } from '../config'
import type { ChatState, CompanyProfile, Material, MaterialsPage, Source } from '../types'
import { ApiError } from '../types'

const TOKEN_KEY = 'predictboss.access_token'
const SESSION_KEY = 'predictboss.session'
const COMPANY_KEY = 'predictboss.company_id'

type CompanyResponse = {
  companyId: string
  profileRevision: number
  profile: Omit<CompanyProfile, 'updatedAt' | 'companyId' | 'profileRevision'>
}
type SourceResponse = {
  sourceId: string
  url: string
  name: string | null
  kind: Source['type']
  validationStatus: string
  enabled: boolean
  pollIntervalMinutes: number
  lastSuccessAt: string | null
  lastError?: string | null
  version: number
}
type LinkResponse = {
  articleId: string
  canonicalUrl: string
  materialKind: Material['kind']
  mediaType: string | null
  title: string | null
  publishedAt: string | null
  newsSourceId: string | null
  discoveredAt: string
  processingStatus: string
}
type CompanyList = { items: Array<{ companyId: string }> }
type AuthResult = {
  access_token: string
  user: { userId: string; login: string; displayName: string | null }
}

async function request<T>(method: string, path: string, body?: unknown, extraHeaders?: HeadersInit): Promise<T> {
  const headers = new Headers(extraHeaders)
  if (body !== undefined) headers.set('Content-Type', 'application/json')
  const accessToken = sessionStorage.getItem(TOKEN_KEY)
  if (accessToken) headers.set('Authorization', `Bearer ${accessToken}`)
  let response: Response
  try {
    response = await fetch(`${API_URL}${path}`, {
      method,
      headers,
      body: body === undefined ? undefined : JSON.stringify(body),
    })
  } catch {
    throw new ApiError('network_error', 'Нет связи с сервером. Проверьте подключение.')
  }
  if (response.status === 204) return undefined as T
  const data: unknown = await response.json().catch(() => null)
  if (!response.ok) {
    const error = (data ?? {}) as { error?: string; code?: string; message?: string }
    if (response.status === 401 && path.startsWith('/api/v1/')) clearSession()
    throw new ApiError(
      error.error ?? error.code ?? 'http_error',
      error.message ?? `Сервер ответил ошибкой ${response.status}.`,
      response.status,
    )
  }
  return data as T
}

function clearSession() {
  sessionStorage.removeItem(TOKEN_KEY)
  sessionStorage.removeItem(SESSION_KEY)
  sessionStorage.removeItem(COMPANY_KEY)
}

async function getCompanies(): Promise<CompanyList['items']> {
  const result = await request<CompanyList>('GET', '/api/v1/companies')
  return result.items ?? []
}

async function currentCompany(): Promise<CompanyResponse | null> {
  const companies = await getCompanies()
  const companyId = companies[0]?.companyId
  if (!companyId) {
    sessionStorage.removeItem(COMPANY_KEY)
    return null
  }
  sessionStorage.setItem(COMPANY_KEY, companyId)
  return request<CompanyResponse>('GET', `/api/v1/companies/${encodeURIComponent(companyId)}`)
}

function profileFrom(response: CompanyResponse): CompanyProfile {
  return { ...response.profile, companyId: response.companyId, profileRevision: response.profileRevision, updatedAt: null }
}

function sourceFrom(response: SourceResponse): Source {
  const status: Source['status'] = !response.enabled
    ? 'paused'
    : response.validationStatus === 'active' || response.validationStatus === 'blocked' || response.validationStatus === 'unsupported'
      ? response.validationStatus
      : 'checking'
  return {
    id: response.sourceId,
    url: response.url,
    name: response.name,
    type: response.kind,
    status,
    statusReason: response.lastError ?? null,
    lastPolledAt: response.lastSuccessAt,
    pollIntervalMinutes: response.pollIntervalMinutes,
    version: response.version,
  }
}

function materialFrom(response: LinkResponse): Material {
  const status: Material['status'] = response.processingStatus === 'accepted'
    ? 'accepted'
    : response.processingStatus === 'retry'
      ? 'retrying'
      : response.processingStatus === 'failed'
        ? 'error'
        : response.processingStatus === 'unsupported' || response.processingStatus === 'skipped'
          ? 'unsupported'
          : 'pending'
  const format = response.mediaType === 'application/pdf' || response.canonicalUrl.toLowerCase().split('?')[0].endsWith('.pdf') ? 'pdf' : 'html'
  return {
    id: response.articleId,
    title: response.title ?? response.canonicalUrl,
    url: response.canonicalUrl,
    publishedAt: response.publishedAt,
    createdAt: response.discoveredAt,
    kind: response.materialKind,
    format,
    origin: response.newsSourceId ? 'regular_source' : 'one_time_link',
    status,
  }
}

async function saveAuth(path: '/auth/login' | '/auth/register', credentials: { login: string; password: string }) {
  const result = await request<AuthResult>('POST', path, credentials)
  sessionStorage.setItem(TOKEN_KEY, result.access_token)
  const companies = await getCompanies()
  const session = { userId: result.user.userId, login: result.user.login, hasCompany: companies.length > 0 }
  sessionStorage.setItem(SESSION_KEY, JSON.stringify(session))
  if (companies[0]) sessionStorage.setItem(COMPANY_KEY, companies[0].companyId)
  return session
}

export const httpApi: Api = {
  getSession: async () => {
    const raw = sessionStorage.getItem(SESSION_KEY)
    if (!sessionStorage.getItem(TOKEN_KEY) || !raw) return null
    try {
      const session = JSON.parse(raw) as { userId: string; login: string; hasCompany: boolean }
      const companies = await getCompanies()
      const updated = { ...session, hasCompany: companies.length > 0 }
      sessionStorage.setItem(SESSION_KEY, JSON.stringify(updated))
      if (companies[0]) sessionStorage.setItem(COMPANY_KEY, companies[0].companyId)
      return updated
    } catch (error) {
      if (error instanceof ApiError && error.status === 401) return null
      throw error
    }
  },
  login: (credentials) => saveAuth('/auth/login', credentials),
  register: (credentials) => saveAuth('/auth/register', credentials),
  logout: async () => clearSession(),

  getProfile: async () => {
    const company = await currentCompany()
    return company ? profileFrom(company) : null
  },
  saveProfile: async (input) => {
    const companyId = sessionStorage.getItem(COMPANY_KEY)
    if (!companyId) {
      const created = await request<CompanyResponse>('POST', '/api/v1/companies', { mode: 'manual', profile: input })
      sessionStorage.setItem(COMPANY_KEY, created.companyId)
      return profileFrom(created)
    }
    const current = await request<CompanyResponse>('GET', `/api/v1/companies/${encodeURIComponent(companyId)}`)
    const updated = await request<CompanyResponse>(
      'PATCH',
      `/api/v1/companies/${encodeURIComponent(companyId)}`,
      input,
      { 'If-Match': `"${current.profileRevision}"` },
    )
    return profileFrom(updated)
  },
  importProfile: async (syntheticU) => {
    if (sessionStorage.getItem(COMPANY_KEY)) {
      throw new ApiError('company_already_exists', 'Компания уже создана; её данные можно изменить в профиле.', 409)
    }
    const created = await request<CompanyResponse>('POST', '/api/v1/companies', {
      mode: 'dataset', syntheticU: syntheticU.trim(),
    })
    sessionStorage.setItem(COMPANY_KEY, created.companyId)
    return profileFrom(created)
  },

  getSources: async () => {
    const companyId = sessionStorage.getItem(COMPANY_KEY) ?? (await currentCompany())?.companyId
    if (!companyId) return []
    const result = await request<{ items: SourceResponse[] }>('GET', `/api/v1/companies/${companyId}/sources`)
    return result.items.map(sourceFrom)
  },
  addSource: async (url) => {
    const companyId = sessionStorage.getItem(COMPANY_KEY)
    if (!companyId) throw new ApiError('company_profile_required', 'Сначала заполните профиль компании.', 409)
    const result = await request<SourceResponse>('POST', `/api/v1/companies/${companyId}/sources`, { url })
    return sourceFrom(result)
  },
  setSourcePaused: async (id, paused) => {
    const sources = await httpApi.getSources()
    const source = sources.find((item) => item.id === id)
    const companyId = sessionStorage.getItem(COMPANY_KEY)
    if (!source || !companyId) throw new ApiError('source_not_found', 'Источник не найден.', 404)
    const result = await request<SourceResponse>(
      'PATCH',
      `/api/v1/companies/${companyId}/sources/${id}`,
      { enabled: !paused },
      { 'If-Match': `"${source.version ?? 1}"` },
    )
    return sourceFrom(result)
  },
  deleteSource: async (id) => {
    const companyId = sessionStorage.getItem(COMPANY_KEY)
    if (!companyId) throw new ApiError('company_profile_required', 'Профиль компании не найден.', 404)
    await request('DELETE', `/api/v1/companies/${companyId}/sources/${id}`)
  },

  getMaterials: async ({ from, to, limit }): Promise<MaterialsPage> => {
    const companyId = sessionStorage.getItem(COMPANY_KEY)
    if (!companyId) return { items: [], hasMore: false }
    const query = new URLSearchParams({ from, to, limit: String(Math.min(limit, 100)) })
    const result = await request<{ items: LinkResponse[]; hasMore: boolean }>(
      'GET', `/api/v1/companies/${companyId}/news?${query}`,
    )
    return { items: result.items.map(materialFrom), hasMore: result.hasMore }
  },
  addMaterial: async (url) => {
    const companyId = sessionStorage.getItem(COMPANY_KEY)
    if (!companyId) throw new ApiError('company_profile_required', 'Сначала заполните профиль компании.', 409)
    let materialKind: Material['kind'] = 'external_news'
    try {
      if (new URL(url).hostname === 'alfabank.ru' || new URL(url).hostname.endsWith('.alfabank.ru')) materialKind = 'alpha_analysis'
    } catch {
      throw new ApiError('invalid_url', 'Введите корректную ссылку.')
    }
    const result = await request<LinkResponse>(
      'POST', `/api/v1/companies/${companyId}/news-links`, { url, materialKind },
    )
    return materialFrom(result)
  },

  getChat: () => request<ChatState>('GET', '/api/v1/chat'),
  sendMessage: (text) => {
    const analysisCommand = /\b(запусти|начни|обнови|проведи)\s+анализ\b|\bпроанализируй\b/i.test(text)
    return analysisCommand
      ? request<ChatState>('POST', '/api/v1/chat/analysis', {})
      : request<ChatState>('POST', '/api/v1/chat/messages', { text })
  },
}
