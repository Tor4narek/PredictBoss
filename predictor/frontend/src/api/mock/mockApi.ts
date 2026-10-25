import type { Api } from '../api'
import {
  ApiError,
  type AnalystMessage,
  type ChatState,
  type CompanyProfile,
  type Material,
  type Session,
  type Source,
} from '../types'
import { demoAnalysis, seedMaterials, seedMessages, seedProfile, seedSources } from './data'

/*
 * Моки бэкенда. Данные живут в localStorage, поэтому переживают перезагрузку страницы.
 *
 * Сценарии для проверки состояний — параметр ?mock= в адресе страницы:
 *   ?mock=empty — пустые списки, профиля нет, диалог пустой;
 *   ?mock=error — каждый запрос завершается ошибкой;
 *   ?mock=slow  — ответы приходят через 3 секунды.
 * Сбросить демо-данные: localStorage.removeItem('pb.mock') в консоли браузера.
 */

type MockDb = {
  session: Session | null
  profile: CompanyProfile | null
  sources: Source[]
  materials: Material[]
  messages: ChatState['messages']
  lastAnalysisAt: string | null
}

const DB_KEY = 'pb.mock'

function seedDb(): MockDb {
  return {
    session: null,
    profile: seedProfile,
    sources: seedSources,
    materials: seedMaterials,
    messages: seedMessages,
    lastAnalysisAt: seedMessages[0].createdAt,
  }
}

function loadDb(): MockDb {
  try {
    const raw = localStorage.getItem(DB_KEY)
    if (raw) return JSON.parse(raw) as MockDb
  } catch {
    // Повреждённые данные — начинаем заново.
  }
  return seedDb()
}

let db = loadDb()

function saveDb() {
  try {
    localStorage.setItem(DB_KEY, JSON.stringify(db))
  } catch {
    // localStorage недоступен — работаем только в памяти.
  }
}

function scenario(): 'empty' | 'error' | 'slow' | null {
  const value = new URLSearchParams(window.location.search).get('mock')
  return value === 'empty' || value === 'error' || value === 'slow' ? value : null
}

/** Имитирует сетевой запрос: задержка и, в сценарии error, ошибка. */
async function respond<T>(getValue: () => T, delayMs = 500): Promise<T> {
  const mode = scenario()
  await new Promise((resolve) => setTimeout(resolve, mode === 'slow' ? 3000 : delayMs))
  if (mode === 'error') {
    throw new ApiError('mock_error', 'Сервер не ответил (тестовый сценарий ?mock=error).', 500)
  }
  return structuredClone(getValue())
}

const isEmpty = () => scenario() === 'empty'

function newId(prefix: string) {
  return `${prefix}-${Math.random().toString(36).slice(2, 10)}`
}

function requireHttps(url: string) {
  let parsed: URL
  try {
    parsed = new URL(url)
  } catch {
    throw new ApiError('invalid_url', 'Введите адрес целиком, например https://example.org/news')
  }
  if (parsed.protocol !== 'https:') {
    throw new ApiError('https_required', 'Поддерживаются только адреса, начинающиеся с https://')
  }
}

function chatState(): ChatState {
  if (isEmpty()) {
    return { lastAnalysisAt: null, hasSources: false, hasProfile: false, messages: [] }
  }
  return {
    lastAnalysisAt: db.lastAnalysisAt,
    hasSources: db.sources.length > 0,
    hasProfile: db.profile !== null,
    messages: db.messages,
  }
}

function analystReply(text: string): AnalystMessage {
  const now = new Date().toISOString()
  if (/запусти\s+анализ/i.test(text)) {
    return {
      id: newId('msg'),
      role: 'analyst',
      createdAt: now,
      text: null,
      analysis: demoAnalysis,
      evidence: [],
      note: null,
    }
  }
  return {
    id: newId('msg'),
    role: 'analyst',
    createdAt: now,
    text:
      'Это демонстрационный ответ: настоящий Константин подключится вместе с API. ' +
      'Он отвечает на вопросы по профилю компании и материалам из источников, а каждое утверждение подкрепляет ссылкой.',
    analysis: null,
    evidence: [],
    note: 'Демонстрационный ответ, не реальный анализ.',
  }
}

export const mockApi: Api = {
  // Сессию сценарии ?mock= не трогают, чтобы состояния страниц можно было смотреть после входа.
  getSession: () => Promise.resolve(structuredClone(db.session)),

  login: ({ login, password }) =>
    respond(() => {
      if (!login.trim() || !password) {
        throw new ApiError('invalid_credentials', 'Введите логин и пароль.')
      }
      db.session = { userId: 'user-1', login: login.trim(), hasCompany: db.profile !== null }
      saveDb()
      return db.session
    }),

  register: ({ login, password }) =>
    respond(() => {
      if (!login.trim() || !password) {
        throw new ApiError('invalid_credentials', 'Введите логин и пароль.')
      }
      db = { ...seedDb(), profile: null, sources: [], materials: [], messages: [], lastAnalysisAt: null }
      db.session = { userId: 'user-1', login: login.trim(), hasCompany: false }
      saveDb()
      return db.session
    }),

  logout: () =>
    respond(() => {
      db.session = null
      saveDb()
    }, 200),

  getProfile: () => respond(() => (isEmpty() ? null : db.profile)),

  saveProfile: (input) =>
    respond(() => {
      db.profile = { ...input, updatedAt: new Date().toISOString() }
      if (db.session) db.session.hasCompany = true
      saveDb()
      return db.profile
    }, 700),

  getSources: () => respond(() => (isEmpty() ? [] : db.sources)),

  addSource: (url) =>
    respond(() => {
      requireHttps(url)
      if (db.sources.some((source) => source.url === url)) {
        throw new ApiError('source_exists', 'Этот источник уже добавлен.')
      }
      const source: Source = {
        id: newId('src'),
        url,
        name: null,
        type: null,
        status: 'checking',
        statusReason: null,
        lastPolledAt: null,
        pollIntervalMinutes: 15,
      }
      db.sources = [source, ...db.sources]
      saveDb()
      return source
    }, 700),

  setSourcePaused: (id, paused) =>
    respond(() => {
      const source = db.sources.find((item) => item.id === id)
      if (!source) throw new ApiError('source_not_found', 'Источник не найден.', 404)
      source.status = paused ? 'paused' : 'active'
      saveDb()
      return source
    }),

  deleteSource: (id) =>
    respond(() => {
      db.sources = db.sources.filter((item) => item.id !== id)
      saveDb()
    }),

  getMaterials: ({ from, to, limit }) =>
    respond(() => {
      if (isEmpty()) return { items: [], hasMore: false }
      const fromMs = new Date(`${from}T00:00:00+03:00`).getTime()
      const toMs = new Date(`${to}T23:59:59+03:00`).getTime()
      const filtered = db.materials
        .filter((item) => {
          const time = new Date(item.publishedAt ?? item.createdAt).getTime()
          return time >= fromMs && time <= toMs
        })
        .sort((a, b) => b.createdAt.localeCompare(a.createdAt))
      return { items: filtered.slice(0, limit), hasMore: filtered.length > limit }
    }),

  addMaterial: (url) =>
    respond(() => {
      requireHttps(url)
      const material: Material = {
        id: newId('mat'),
        title: new URL(url).pathname.split('/').filter(Boolean).pop() ?? url,
        url,
        publishedAt: null,
        createdAt: new Date().toISOString(),
        kind: url.includes('alfabank.ru') ? 'alpha_analysis' : 'external_news',
        format: url.toLowerCase().endsWith('.pdf') ? 'pdf' : 'html',
        origin: 'one_time_link',
        status: 'pending',
      }
      db.materials = [material, ...db.materials]
      saveDb()
      return material
    }, 700),

  getChat: () => respond(chatState),

  sendMessage: (text) =>
    respond(() => {
      const now = new Date().toISOString()
      db.messages = [...db.messages, { id: newId('msg'), role: 'user', createdAt: now, text }]
      const reply = analystReply(text)
      db.messages = [...db.messages, reply]
      if (reply.analysis) db.lastAnalysisAt = reply.createdAt
      saveDb()
      return chatState()
    }, 1500),
}
