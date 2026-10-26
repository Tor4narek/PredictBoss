import type {
  BusinessDevelopmentStage,
  ClientType,
  CompanyAgeGradation,
  EmployeeCountGradation,
  OrganizationType,
  OwnershipForm,
} from './companyEnums'

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
  companyId?: string
  profileRevision?: number
  /** Идентификатор строки из CSV; задаётся только при импорте. */
  syntheticU: string | null
  orgType: OrganizationType | null
  ownershipForm: OwnershipForm | null
  companyAgeGradation: CompanyAgeGradation | null
  developmentStage: BusinessDevelopmentStage | null
  mainOkvedCode: string | null
  mainOkvedName: string | null
  negativeMark: boolean | null
  workersFnsGradation: EmployeeCountGradation | null
  workersSparkGradation: EmployeeCountGradation | null
  clientType: ClientType | null
  hasRko: boolean | null
  hasGuarantees: boolean | null
  hasLoans: boolean | null
  hasLeasing: boolean | null
  hasDeposits: boolean | null
  hasPayroll: boolean | null
  ftsFlag: boolean | null
  ftsCountryName: string | null
  hasFactoring: boolean | null
  /** Годовая выручка в рублях. */
  revenueCompany: number | null
  groupId: string | null
  city: string | null
  region: string | null
  updatedAt: string | null
}

export type CompanyProfileInput = Omit<CompanyProfile, 'companyId' | 'profileRevision' | 'syntheticU' | 'updatedAt'>

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
  version?: number
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
/** Ссылка-доказательство. index — номер в квадратных скобках в тексте: «Факт [1]». */
export type Evidence = {
  index: number
  sourceId: string
  title: string
  url: string
  /** Пояснение происхождения доказательства. */
  note: string | null
}

export type Analysis = {
  title: string
  /** Строка под заголовком: «Событие: … · 25 октября 2026». */
  subtitle: string
  /** Неизменённый текст карточки анализа из ESB. */
  text: string
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
  processing: boolean
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
