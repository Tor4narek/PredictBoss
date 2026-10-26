import { Link, Outlet } from 'react-router'
import brandMark from '../assets/brand-mark.svg'
import styles from './AuthLayout.module.css'

/** Каркас экранов входа: логотип сверху, карточка по центру (фреймы «01 · Вход»). */
export function AuthLayout() {
  return (
    <div className={styles.layout}>
      <header className={styles.brand}>
        <Link to="/" className={styles.brandLink}>
          <img src={brandMark} width={40} height={40} alt="" />
          <span>Альфа Пульс</span>
        </Link>
      </header>
      <main className={styles.main}>
        <Outlet />
      </main>
    </div>
  )
}
