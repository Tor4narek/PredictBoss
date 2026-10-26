import type { ReactNode } from 'react'
import { cx } from '../cx'
import styles from './PageHeader.module.css'

export type PageHeaderProps = {
  /** Заголовок страницы (Heading L, 28). */
  title: ReactNode
  /** Серый подзаголовок. */
  description?: ReactNode
  /** Кнопки или чипы справа. */
  actions?: ReactNode
  className?: string
}

/** Шапка страницы: заголовок, подзаголовок и действия справа. */
export function PageHeader({ title, description, actions, className }: PageHeaderProps) {
  return (
    <header className={cx(styles.header, className)}>
      <div className={styles.headings}>
        <h1 className={styles.title}>{title}</h1>
        {description !== undefined && <p className={styles.description}>{description}</p>}
      </div>
      {actions !== undefined && <div className={styles.actions}>{actions}</div>}
    </header>
  )
}
