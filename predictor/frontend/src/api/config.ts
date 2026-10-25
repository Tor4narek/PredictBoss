/** true — данные из моков (по умолчанию), false — из сервиса Domain. Задаётся в .env. */
export const USE_MOCKS = import.meta.env.VITE_USE_MOCKS !== 'false'

/** Адрес сервиса Domain. */
export const API_URL = (import.meta.env.VITE_API_URL ?? '').replace(/\/$/, '')
