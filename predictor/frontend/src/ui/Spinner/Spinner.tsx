import { cx } from '../cx'
import styles from './Spinner.module.css'

export type SpinnerProps = {
  /** Диаметр в пикселях. */
  size?: number
  /** Подпись для экранных читалок. */
  label?: string
  className?: string
}

/** Индикатор загрузки. Цвет берёт от текста родителя (currentColor). */
export function Spinner({ size = 20, label = 'Загрузка', className }: SpinnerProps) {
  return (
    <span
      role="status"
      aria-label={label}
      className={cx(styles.spinner, className)}
      style={{ width: size, height: size }}
    />
  )
}
