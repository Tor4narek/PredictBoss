import type { HTMLAttributes, ReactNode } from 'react'
import { cx } from '../cx'
import styles from './Panel.module.css'

export type PanelProps = Omit<HTMLAttributes<HTMLElement>, 'title'> & {
  /** Заголовок карточки (18, жирный). */
  title?: ReactNode
  /** Серый подзаголовок под заголовком. */
  description?: ReactNode
  /** Элемент справа от заголовка: бейдж или кнопка. */
  aside?: ReactNode
  /** Внутренний отступ: md — 16 (плашки), lg — 24 (обычные карточки), xl — 32 (формы), none — без отступа. */
  padding?: 'none' | 'md' | 'lg' | 'xl'
  children?: ReactNode
}

/** Белая карточка с рамкой и скруглением 12 — общий контейнер со слотом для содержимого. */
export function Panel({
  title,
  description,
  aside,
  padding = 'lg',
  className,
  children,
  ...rest
}: PanelProps) {
  const hasHeader = title !== undefined || description !== undefined || aside !== undefined

  return (
    <section className={cx(styles.panel, styles[`padding-${padding}`], className)} {...rest}>
      {hasHeader && (
        <header className={styles.header}>
          <div className={styles.headings}>
            {title !== undefined && <h2 className={styles.title}>{title}</h2>}
            {description !== undefined && <p className={styles.description}>{description}</p>}
          </div>
          {aside}
        </header>
      )}
      {children}
    </section>
  )
}
