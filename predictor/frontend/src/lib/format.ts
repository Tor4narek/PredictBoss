/* Форматирование дат, чисел и интервалов для интерфейса. Время показываем по Москве. */

const TIME_ZONE = 'Europe/Moscow'

const dateTimeFormat = new Intl.DateTimeFormat('ru-RU', {
  timeZone: TIME_ZONE,
  day: '2-digit',
  month: '2-digit',
  year: 'numeric',
  hour: '2-digit',
  minute: '2-digit',
})

const shortDateTimeFormat = new Intl.DateTimeFormat('ru-RU', {
  timeZone: TIME_ZONE,
  day: '2-digit',
  month: '2-digit',
  hour: '2-digit',
  minute: '2-digit',
})

const longDateFormat = new Intl.DateTimeFormat('ru-RU', {
  timeZone: TIME_ZONE,
  day: 'numeric',
  month: 'long',
  year: 'numeric',
})

const timeFormat = new Intl.DateTimeFormat('ru-RU', {
  timeZone: TIME_ZONE,
  hour: '2-digit',
  minute: '2-digit',
})

/** «24.10.2026, 09:15 МСК» */
export function formatDateTime(iso: string): string {
  return `${dateTimeFormat.format(new Date(iso))} МСК`
}

/** «25.10, 08:45 МСК» */
export function formatShortDateTime(iso: string): string {
  return `${shortDateTimeFormat.format(new Date(iso))} МСК`
}

/** «25 октября 2026, 09:00 МСК» */
export function formatLongDateTime(iso: string): string {
  const date = longDateFormat.format(new Date(iso)).replace(/\s*г\.$/, '')
  return `${date}, ${timeFormat.format(new Date(iso))} МСК`
}

/** «15 мин», «1 час», «3 часа» */
export function formatInterval(minutes: number): string {
  if (minutes < 60) return `${minutes} мин`
  const hours = Math.round(minutes / 60)
  return `${hours} ${plural(hours, 'час', 'часа', 'часов')}`
}

/** «18 000 000 000» */
export function formatNumber(value: number): string {
  return new Intl.NumberFormat('ru-RU').format(value).replace(/[^\d,-]/g, ' ')
}

/** Дата в формате поля type="date" (YYYY-MM-DD) по Москве. */
export function toDateInputValue(date: Date): string {
  return new Intl.DateTimeFormat('en-CA', { timeZone: TIME_ZONE }).format(date)
}

/** Адрес без https:// — как показан на макетах. */
export function shortUrl(url: string): string {
  return url.replace(/^https?:\/\//, '')
}

function plural(n: number, one: string, few: string, many: string): string {
  const mod10 = n % 10
  const mod100 = n % 100
  if (mod10 === 1 && mod100 !== 11) return one
  if (mod10 >= 2 && mod10 <= 4 && (mod100 < 12 || mod100 > 14)) return few
  return many
}
