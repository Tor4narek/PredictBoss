import { cx } from '../cx'
import styles from './Logo.module.css'

export type LogoProps = {
  /** Показать подпись «Новостная аналитика». */
  withSubtitle?: boolean
  className?: string
}

/** Логотип «Альфа Пульс»: красный квадрат с буквой «А» и название. */
export function Logo({ withSubtitle = true, className }: LogoProps) {
  return (
    <div className={cx(styles.logo, className)}>
      <span className={styles.mark} aria-hidden="true">
        А
      </span>
      <span className={styles.text}>
        <span className={styles.title}>Альфа Пульс</span>
        {withSubtitle && <span className={styles.subtitle}>Новостная аналитика</span>}
      </span>
    </div>
  )
}
