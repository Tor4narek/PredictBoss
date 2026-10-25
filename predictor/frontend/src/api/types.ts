/*
 * Форматы данных, которые фронтенд ожидает от сервиса Domain.
 * Время — строка ISO 8601 с часовым поясом. Неизвестное значение — null.
 * Названия полей профиля совпадают с company_data у агента Валеры.
 */

// ——— Пользователь и вход ———

export type Session = {
  userId: string
  login: string
  /** Есть ли у пользователя заполненный профиль компании. Без него показываем мастер создания. */
  hasCompany: boolean
}

export type Credentials = {
  login: string
  password: string
}

// ——— Профиль компании ———

export type CompanyProfile = {
  /** Тип организации, например «Юридическое лицо». */
  orgType: string | null
  /** Код основного ОКВЭД, например «46.90». */
  mainOkvedCode: string | null
  /** Название основного ОКВЭД. */
  mainOkvedName: string | null
  region: string | null
  /** Годовая выручка в рублях. */
  revenueCompany: number | null
  /** Есть ли кредиты. */
  hasLoans: boolean | null
  /** Когда профиль последний раз сохранён. */
  updatedAt: string | null
}

export type CompanyProfileInput = Omit<CompanyProfile, 'updatedAt'>

// ——— Регулярные источники ———

export type SourceStatus = 'active' | 'checking' | 'paused' | 'blocked' | 'unsupported'

/** Тип определяет сервер по содержимому ссылки. */
export type SourceType = 'html' | 'rss' | 'atom' | 'sitemap'

export type Source = {
  id: string
  url: string
  /** Название сайта, которое определил сервер. */
  name: string | null
  type: SourceType | null
  status: SourceStatus
  /** Пояснение к статусу: «Обход запрещён robots.txt», «Не найдены статьи». */
  statusReason: string | null
  lastPolledAt: string | null
  /** Интервал опроса в минутах, его задаёт сервер. */
  pollIntervalMinutes: number | null
}

// ——— Материалы (новости и анализы банка) ———

export type MaterialKind = 'external_news' | 'alpha_analysis'
export type MaterialFormat = 'html' | 'pdf'
export type MaterialOrigin = 'regular_source' | 'one_time_link'
export type MaterialStatus = 'accepted' | 'pending' | 'error' | 'unsupported' | 'retrying'

export type Material = {
  id: string
  title: string
  url: string
  publishedAt: string | null
  /** Когда материал загружен. По нему фильтруем, если дата публикации неизвестна. */
  createdAt: string
  kind: MaterialKind
  format: MaterialFormat
  origin: MaterialOrigin
  status: MaterialStatus
}

export type MaterialsQuery = {
  /** Начало периода включительно, YYYY-MM-DD. */
  from: string
  /** Конец периода включительно, YYYY-MM-DD. */
  to: string
  limit: number
}

export type MaterialsPage = {
  items: Material[]
  hasMore: boolean
}

// ——— Диалог с Константином ———

/** Раздел карточки анализа. originKind совпадает с ESB: fact, bank_view, ai_inference. */
export type AnalysisSection = {
  kind: 'fact' | 'bank_view' | 'ai_inference' | 'financial_effect' | 'action'
  title: string
  text: string
}

/** Ссылка-доказательство. index — номер в квадратных скобках в тексте: «Факт [1]». */
export type Evidence = {
  index: number
  sourceId: string
  title: string
  url: string
  /** Пометка под ссылкой, например «демонстрационный материал». */
  note: string | null
}

export type Analysis = {
  title: string
  /** Строка под заголовком: «Событие: … · 25 октября 2026». */
  subtitle: string
  sections: AnalysisSection[]
  evidence: Evidence[]
}

export type UserMessage = {
  id: string
  role: 'user'
  createdAt: string
  text: string
}

export type AnalystMessage = {
  id: string
  role: 'analyst'
  createdAt: string
  /** Обычный ответ текстом. */
  text: string | null
  /** Полная карточка анализа (первое сообщение после «Запусти анализ»). */
  analysis: Analysis | null
  evidence: Evidence[]
  /** Мелкая пометка под ответом. */
  note: string | null
}

export type ChatMessage = UserMessage | AnalystMessage

export type ChatState = {
  /** Когда был последний анализ; null — анализа ещё не было. */
  lastAnalysisAt: string | null
  /** Есть ли хотя бы один источник (для подсказок в пустом чате). */
  hasSources: boolean
  hasProfile: boolean
  messages: ChatMessage[]
}

// ——— Ошибки ———

/** Ошибка API в формате { "error": "stable_code", "message": "Текст для человека" }. */
export class ApiError extends Error {
  readonly code: string
  readonly status: number

  constructor(code: string, message: string, status = 0) {
    super(message)
    this.name = 'ApiError'
    this.code = code
    this.status = status
  }
}
