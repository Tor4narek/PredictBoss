import type { Api } from './api'
import { USE_MOCKS } from './config'
import { httpApi } from './http/httpApi'
import { mockApi } from './mock/mockApi'

/** Единая точка доступа к данным. Страницы используют хуки из ./hooks, а не fetch напрямую. */
export const api: Api = USE_MOCKS ? mockApi : httpApi

/** Показывать ли плашки «Демонстрационные данные». */
export const isDemoData = USE_MOCKS

export type { Api } from './api'
export * from './types'
