import type { MaterialKind, MaterialOrigin, MaterialStatus } from '../../api'
import type { BadgeTone } from '../../ui'

export const MATERIAL_STATUS: Record<MaterialStatus, { label: string; tone: BadgeTone }> = {
  accepted: { label: 'Принято ESB', tone: 'success' },
  pending: { label: 'Ожидает обработки', tone: 'warning' },
  error: { label: 'Ошибка', tone: 'accent' },
  unsupported: { label: 'Не поддерживается', tone: 'neutral' },
  retrying: { label: 'Повторная попытка', tone: 'warning' },
}

export const MATERIAL_KIND: Record<MaterialKind, string> = {
  external_news: 'Внешняя новость',
  alpha_analysis: 'Анализ Альфа-Банка',
}

export const MATERIAL_ORIGIN: Record<MaterialOrigin, string> = {
  regular_source: 'Регулярный источник',
  one_time_link: 'Разовая ссылка',
}
