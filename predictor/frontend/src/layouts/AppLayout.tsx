import { LogOut } from 'lucide-react'
import { Outlet, useNavigate } from 'react-router'
import { useLogout } from '../api/hooks'
import { Button, Sidebar } from '../ui'
import styles from './AppLayout.module.css'

/** Каркас приложения: боковое меню слева, страница справа. */
export function AppLayout() {
  const logout = useLogout()
  const navigate = useNavigate()

  return (
    <div className={styles.layout}>
      <Sidebar
        className={styles.sidebar}
        footer={
          <Button
            variant="ghost"
            fullWidth
            className={styles.logout}
            icon={<LogOut size={20} aria-hidden="true" />}
            loading={logout.isPending}
            onClick={() => logout.mutate(undefined, { onSuccess: () => navigate('/') })}
          >
            Выйти
          </Button>
        }
      />
      <main className={styles.main}>
        <Outlet />
      </main>
    </div>
  )
}
