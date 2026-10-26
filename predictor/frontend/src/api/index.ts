import type { Api } from './api'
import { httpApi } from './http/httpApi'

/** Все страницы используют реальные endpoint-ы Domain API. */
export const api: Api = httpApi

export { isAnalysisCommand } from './api'
export type { Api } from './api'
export * from './types'
