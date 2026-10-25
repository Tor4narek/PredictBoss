import { cx } from '../cx'
import { Logo } from '../Logo/Logo'
import { NavItem } from '../NavItem/NavItem'
import { DEFAULT_SIDEBAR_ITEMS, type SidebarItem } from './sidebarItems'
import styles from './Sidebar.module.css'

export type SidebarProps = {
  items?: SidebarItem[]
  /** Принудительно подсветить пункт (для витрины /kit). Обычно берётся из адреса страницы. */
  activeTo?: string
  className?: string
}

/** Левая панель шириной 260: логотип и меню. */
export function Sidebar({ items = DEFAULT_SIDEBAR_ITEMS, activeTo, className }: SidebarProps) {
  return (
    <aside className={cx(styles.sidebar, className)}>
      <Logo />
      <nav className={styles.nav}>
        {items.map((item) => (
          <NavItem
            key={item.to}
            {...item}
            active={activeTo === undefined ? undefined : activeTo === item.to}
          />
        ))}
      </nav>
    </aside>
  )
}
