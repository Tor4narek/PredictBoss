import { Link, useNavigate } from 'react-router'
import { Button } from '../../ui'
import styles from './AuthPages.module.css'

/** Первый экран для гостя (фрейм «01 · Вход», вариант с описанием продукта). */
export function LandingPage() {
  const navigate = useNavigate()

  return (
    <section className={styles.card}>
      <div className={styles.about}>
        <p className={styles.eyebrow}>Аналитика для руководителей</p>
        <h1 className={styles.heroTitle}>Новости в контексте вашего бизнеса</h1>
        <p className={styles.heroText}>
          Понимайте, как внешние события влияют на компанию. Опирайтесь на проверяемые
          доказательства и задавайте уточняющие вопросы Константину.
        </p>
      </div>
      <div className={styles.actions}>
        <Button size="lg" fullWidth onClick={() => navigate('/login')}>
          Зарегистрироваться/Войти
        </Button>
        {/* Вход через систему авторизации приложения (SSO) пока не подключён — ведёт на обычный вход. */}
        <p className={styles.switch}>
          или <Link to="/login">через систему авторизации приложения</Link>
        </p>
      </div>
    </section>
  )
}
