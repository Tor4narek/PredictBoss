import type { ReactNode } from 'react'
import { CircleAlert, Inbox } from 'lucide-react'
import { Button } from '../Button/Button'
import { Spinner } from '../Spinner/Spinner'
import { cx } from '../cx'
import styles from './StatusMessage.module.css'

/*
 * Состояния «загрузка», «пусто» и «ошибка» для страниц и таблиц.
 * В Figma их нет, поэтому они собраны из тех же токенов и компонентов.
 */

export type LoadingStateProps = {
  label?: string
  className?: string
}

export function LoadingState({ label = 'Загружаем данные…', className }: LoadingStateProps) {
  return (
    <div className={cx(styles.box, className)}>
      <Spinner size={24} className={styles.spinner} />
      <p className={styles.text}>{label}</p>
    </div>
  )
}

export type EmptyStateProps = {
  title: ReactNode
  description?: ReactNode
  /** Кнопка-подсказка, что делать дальше. */
  action?: ReactNode
  className?: string
}

export function EmptyState({ title, description, action, className }: EmptyStateProps) {
  return (
    <div className={cx(styles.box, className)}>
      <span className={styles.icon}>
        <Inbox size={24} aria-hidden="true" />
      </span>
      <p className={styles.title}>{title}</p>
      {description !== undefined && <p className={styles.text}>{description}</p>}
      {action}
    </div>
  )
}

export type ErrorStateProps = {
  title?: ReactNode
  description?: ReactNode
  /** Если передан, показывается кнопка «Повторить». */
  onRetry?: () => void
  className?: string
}

export function ErrorState({
  title = 'Не удалось загрузить данные',
  description = 'Проверьте соединение и попробуйте ещё раз.',
  onRetry,
  className,
}: ErrorStateProps) {
  return (
    <div className={cx(styles.box, className)} role="alert">
      <span className={cx(styles.icon, styles.iconError)}>
        <CircleAlert size={24} aria-hidden="true" />
      </span>
      <p className={styles.title}>{title}</p>
      <p className={styles.text}>{description}</p>
      {onRetry && (
        <Button variant="secondary" onClick={onRetry}>
          Повторить
        </Button>
      )}
    </div>
  )
}
