import type { ButtonHTMLAttributes, ReactNode } from 'react'
import { cx } from '../cx'
import styles from './ActionLink.module.css'

type CommonProps = {
  children: ReactNode
  className?: string
}

type AsButton = CommonProps &
  Omit<ButtonHTMLAttributes<HTMLButtonElement>, 'children'> & { href?: undefined }

type AsLink = CommonProps & {
  /** Если задан, рендерится ссылка (открывается в новой вкладке). */
  href: string
}

export type ActionLinkProps = AsButton | AsLink

/** Красное текстовое действие в таблице: «Подробнее ↗», «Пауза», «Удалить». */
export function ActionLink(props: ActionLinkProps) {
  if (props.href !== undefined) {
    const { href, children, className } = props
    return (
      <a className={cx(styles.action, className)} href={href} target="_blank" rel="noreferrer">
        {children}
      </a>
    )
  }

  const { children, className, type = 'button', ...rest } = props
  return (
    <button type={type} className={cx(styles.action, className)} {...rest}>
      {children}
    </button>
  )
}
