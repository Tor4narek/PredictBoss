import { useState, type FormEvent } from 'react'
import { Link, useNavigate } from 'react-router'
import { useRegister } from '../../api/hooks'
import { Button, Field } from '../../ui'
import styles from './AuthPages.module.css'

const MIN_PASSWORD_LENGTH = 5

/** Регистрация (фрейм «01 · Вход», вариант «Регистрация»). После неё — мастер создания компании. */
export function RegisterPage() {
  const navigate = useNavigate()
  const register = useRegister()
  const [values, setValues] = useState({ login: '', password: '', repeat: '' })
  const [touched, setTouched] = useState(false)

  const errors = touched
    ? {
        login: !values.login.trim() ? 'Введите логин' : undefined,
        password:
          values.password.length < MIN_PASSWORD_LENGTH
            ? `Пароль — не короче ${MIN_PASSWORD_LENGTH} символов`
            : undefined,
        repeat: values.repeat !== values.password ? 'Пароли не совпадают' : undefined,
      }
    : {}

  function handleSubmit(event: FormEvent) {
    event.preventDefault()
    setTouched(true)
    const invalid =
      !values.login.trim() ||
      values.password.length < MIN_PASSWORD_LENGTH ||
      values.repeat !== values.password
    if (invalid) return
    register.mutate(
      { login: values.login, password: values.password },
      { onSuccess: () => navigate('/onboarding', { replace: true }) },
    )
  }

  return (
    <form className={styles.card} onSubmit={handleSubmit} noValidate>
      <h1 className={styles.title}>Регистрация</h1>
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
          autoComplete="new-password"
          value={values.password}
          onChange={(event) => setValues({ ...values, password: event.target.value })}
          error={errors.password}
        />
        <Field
          label="Повторите пароль"
          type="password"
          autoComplete="new-password"
          value={values.repeat}
          onChange={(event) => setValues({ ...values, repeat: event.target.value })}
          error={errors.repeat}
        />
        {register.isError && (
          <p className={styles.error} role="alert">
            {register.error.message}
          </p>
        )}
      </div>
      <div className={styles.actions}>
        <Button type="submit" size="lg" fullWidth loading={register.isPending}>
          Зарегистрироваться
        </Button>
        <p className={styles.switch}>
          Уже есть аккаунт? <Link to="/login">Войти</Link>
        </p>
      </div>
    </form>
  )
}
