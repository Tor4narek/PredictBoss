import { createBrowserRouter, Navigate, RouterProvider } from 'react-router'
import { AppLayout } from './layouts/AppLayout'
import { AuthLayout } from './layouts/AuthLayout'
import { RedirectIfAuthed, RequireAuth } from './layouts/RouteGuards'
import { LandingPage } from './pages/auth/LandingPage'
import { LoginPage } from './pages/auth/LoginPage'
import { RegisterPage } from './pages/auth/RegisterPage'
import { ChatPage } from './pages/ChatPage/ChatPage'
import { KitPage } from './pages/KitPage/KitPage'
import { MaterialNewPage } from './pages/MaterialNewPage/MaterialNewPage'
import { MaterialsPage } from './pages/MaterialsPage/MaterialsPage'
import { OnboardingPage } from './pages/OnboardingPage/OnboardingPage'
import { ProfilePage } from './pages/ProfilePage/ProfilePage'
import { SourcesPage } from './pages/SourcesPage/SourcesPage'

const router = createBrowserRouter([
  // Гость: лендинг, вход, регистрация.
  {
    element: <RedirectIfAuthed />,
    children: [
      {
        element: <AuthLayout />,
        children: [
          { path: '/', element: <LandingPage /> },
          { path: '/login', element: <LoginPage /> },
          { path: '/register', element: <RegisterPage /> },
        ],
      },
    ],
  },
  // Вошедший пользователь: страницы с боковым меню.
  {
    element: <RequireAuth />,
    children: [
      {
        element: <AppLayout />,
        children: [
          { path: '/onboarding', element: <OnboardingPage /> },
          { path: '/chat', element: <ChatPage /> },
          { path: '/sources', element: <SourcesPage /> },
          { path: '/materials', element: <MaterialsPage /> },
          { path: '/materials/new', element: <MaterialNewPage /> },
          { path: '/profile', element: <ProfilePage /> },
        ],
      },
    ],
  },
  // Витрина UI kit доступна всегда.
  { path: '/kit', element: <KitPage /> },
  { path: '*', element: <Navigate to="/" replace /> },
])

export function App() {
  return <RouterProvider router={router} />
}
