import type { ReactNode } from 'react'
import { cx } from '../cx'
import styles from './Chip.module.css'

export type ChipProps = {
  /** Иконка 24×24 слева. */
  icon?: ReactNode
  children: ReactNode
  className?: string
}

/** Информационный чип-«таблетка» с иконкой: «HTTPS только», «Автопроверка». Не кликабельный. */
export function Chip({ icon, children, className }: ChipProps) {
  return (
    <span className={cx(styles.chip, className)}>
      {icon}
      {children}
    </span>
  )
}
