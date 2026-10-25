import { useState, type FormEvent } from 'react'
import { Link, useLocation, useNavigate } from 'react-router'
import { useLogin } from '../../api/hooks'
import { Button, Field } from '../../ui'
import styles from './AuthPages.module.css'

/** Вход (фрейм «01 · Вход»). Пока на моках: проходит любой логин и пароль. */
export function LoginPage() {
  const navigate = useNavigate()
  const location = useLocation()
  const login = useLogin()
  const [values, setValues] = useState({ login: '', password: '' })
  const [touched, setTouched] = useState(false)

  const errors = {
    login: touched && !values.login.trim() ? 'Введите логин' : undefined,
    password: touched && !values.password ? 'Введите пароль' : undefined,
  }

  function handleSubmit(event: FormEvent) {
    event.preventDefault()
    setTouched(true)
    if (!values.login.trim() || !values.password) return
    login.mutate(values, {
      onSuccess: (session) => {
        const from = (location.state as { from?: string } | null)?.from
        navigate(from ?? (session.hasCompany ? '/chat' : '/onboarding'), { replace: true })
      },
    })
  }

  return (
    <form className={styles.card} onSubmit={handleSubmit} noValidate>
      <h1 className={styles.title}>Вход</h1>
      <div className={styles.fields}>
        <Field
          label="Логин"
          type="email"
          autoComplete="username"
          placeholder="user@mail.com"
          value={values.login}
          onChange={(event) => setValues({ ...values, login: event.target.value })}
          error={errors.login}
        />
        <Field
          label="Пароль"
          type="password"
          autoComplete="current-password"
          value={values.password}
          onChange={(event) => setValues({ ...values, password: event.target.value })}
          error={errors.password}
        />
        {login.isError && (
          <p className={styles.error} role="alert">
            {login.error.message}
          </p>
        )}
      </div>
      <div className={styles.actions}>
        <Button type="submit" size="lg" fullWidth loading={login.isPending}>
          Войти
        </Button>
        <p className={styles.switch}>
          Нет аккаунта? <Link to="/register">Зарегистрироваться</Link>
        </p>
      </div>
    </form>
  )
}
