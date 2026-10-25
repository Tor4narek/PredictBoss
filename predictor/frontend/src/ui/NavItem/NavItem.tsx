import type { LucideIcon } from 'lucide-react'
import { NavLink } from 'react-router'
import { cx } from '../cx'
import styles from './NavItem.module.css'

export type NavItemProps = {
  to: string
  label: string
  icon: LucideIcon
  /**
   * Принудительно задать состояние (для витрины /kit).
   * Если не задано, пункт активен, когда адрес страницы совпадает с `to`.
   */
  active?: boolean
}

/** Пункт бокового меню высотой 44: иконка 20 + подпись. */
export function NavItem({ to, label, icon: Icon, active }: NavItemProps) {
  return (
    <NavLink
      to={to}
      className={({ isActive }) =>
        cx(styles.item, (active ?? isActive) && styles.active)
      }
    >
      <Icon size={20} strokeWidth={2} className={styles.icon} aria-hidden="true" />
      <span>{label}</span>
    </NavLink>
  )
}
