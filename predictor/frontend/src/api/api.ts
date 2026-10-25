import type {
  ChatState,
  CompanyProfile,
  CompanyProfileInput,
  Credentials,
  MaterialsPage,
  MaterialsQuery,
  Material,
  Session,
  Source,
} from './types'

/**
 * Всё, что страницы могут попросить у бэкенда.
 * Есть две реализации с одинаковыми функциями: моки (mock/) и реальные запросы (http/).
 */
export type Api = {
  getSession(): Promise<Session | null>
  login(credentials: Credentials): Promise<Session>
  register(credentials: Credentials): Promise<Session>
  logout(): Promise<void>

  /** null — профиль ещё не заполнен. */
  getProfile(): Promise<CompanyProfile | null>
  saveProfile(input: CompanyProfileInput): Promise<CompanyProfile>

  getSources(): Promise<Source[]>
  addSource(url: string): Promise<Source>
  setSourcePaused(id: string, paused: boolean): Promise<Source>
  deleteSource(id: string): Promise<void>

  getMaterials(query: MaterialsQuery): Promise<MaterialsPage>
  addMaterial(url: string): Promise<Material>

  getChat(): Promise<ChatState>
  /** Отправляет вопрос. Возвращает обновлённый диалог с ответом Константина. */
  sendMessage(text: string): Promise<ChatState>
}
