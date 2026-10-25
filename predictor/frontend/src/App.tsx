import { createBrowserRouter, Navigate, RouterProvider } from 'react-router'
import { KitPage } from './pages/KitPage/KitPage'

// Пока готова только витрина UI kit. Страницы приложения добавятся на Этапе 2.
const router = createBrowserRouter([
  { path: '/kit', element: <KitPage /> },
  { path: '*', element: <Navigate to="/kit" replace /> },
])

export function App() {
  return <RouterProvider router={router} />
}
