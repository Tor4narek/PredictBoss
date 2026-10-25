import { BarChart3, Building2, Database, FileText, type LucideIcon } from 'lucide-react'

export type SidebarItem = {
  to: string
  label: string
  icon: LucideIcon
}

/** Пункты меню из макета. Адреса совпадают с роутами приложения. */
export const DEFAULT_SIDEBAR_ITEMS: SidebarItem[] = [
  { to: '/chat', label: 'Диалог компании', icon: BarChart3 },
  { to: '/sources', label: 'Источники', icon: Database },
  { to: '/materials', label: 'Материалы', icon: FileText },
  { to: '/profile', label: 'Профиль компании', icon: Building2 },
]
