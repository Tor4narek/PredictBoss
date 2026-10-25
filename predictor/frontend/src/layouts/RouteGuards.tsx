import { Navigate, Outlet, useLocation } from 'react-router'
import { useSession } from '../api/hooks'
import { ErrorState, LoadingState } from '../ui'

/** Пускает на страницы приложения только после входа. Иначе — на /login. */
export function RequireAuth() {
  const session = useSession()
  const location = useLocation()

  if (session.isPending) return <LoadingState label="Проверяем вход…" />
  if (session.isError) return <ErrorState onRetry={() => session.refetch()} />
  if (!session.data) return <Navigate to="/login" replace state={{ from: location.pathname }} />
  return <Outlet />
}

/** Экраны входа и регистрации не нужны тому, кто уже вошёл: отправляем в диалог. */
export function RedirectIfAuthed() {
  const session = useSession()

  if (session.isPending) return <LoadingState label="Проверяем вход…" />
  if (session.data) return <Navigate to={session.data.hasCompany ? '/chat' : '/onboarding'} replace />
  return <Outlet />
}
