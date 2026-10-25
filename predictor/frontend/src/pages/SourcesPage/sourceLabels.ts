import type { SourceStatus, SourceType } from '../../api'
import type { BadgeTone } from '../../ui'

export const SOURCE_STATUS: Record<SourceStatus, { label: string; tone: BadgeTone }> = {
  active: { label: 'Активен', tone: 'success' },
  checking: { label: 'Проверяется', tone: 'accent' },
  paused: { label: 'На паузе', tone: 'neutral' },
  blocked: { label: 'Заблокирован', tone: 'accent' },
  unsupported: { label: 'Не поддерживается', tone: 'neutral' },
}

export const SOURCE_TYPE: Record<SourceType, string> = {
  html: 'HTML',
  rss: 'RSS',
  atom: 'Atom',
  sitemap: 'Sitemap',
}
