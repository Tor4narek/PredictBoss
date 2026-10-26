import type { ReactNode } from 'react'
import { cx } from '../cx'
import styles from './Badge.module.css'

export type BadgeTone = 'neutral' | 'accent' | 'success' | 'warning'

export type BadgeProps = {
  /** neutral — серый, accent — красный, success — зелёный, warning — оранжевый. */
  tone?: BadgeTone
  /** Круглый бейдж 28×28 под цифру (номер шага). */
  round?: boolean
  children: ReactNode
  className?: string
}

/** Статус-«таблетка» высотой 28. */
export function Badge({ tone = 'neutral', round = false, children, className }: BadgeProps) {
  return (
    <span className={cx(styles.badge, styles[tone], round && styles.round, className)}>
      {children}
    </span>
  )
}
