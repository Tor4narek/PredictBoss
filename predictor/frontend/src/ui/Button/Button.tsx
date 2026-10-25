import type { ButtonHTMLAttributes, ReactNode } from 'react'
import { cx } from '../cx'
import { Spinner } from '../Spinner/Spinner'
import styles from './Button.module.css'

export type ButtonProps = ButtonHTMLAttributes<HTMLButtonElement> & {
  /** primary — красная, secondary — белая с рамкой, ghost — только текст. */
  variant?: 'primary' | 'secondary' | 'ghost'
  /** md — высота 40, lg — высота 48 (как в Figma). */
  size?: 'md' | 'lg'
  /** Показывает спиннер и блокирует кнопку. */
  loading?: boolean
  /** Растянуть на всю ширину родителя. */
  fullWidth?: boolean
  /** Квадратная кнопка под один символ или иконку (кнопка отправки в поле сообщения). */
  square?: boolean
  icon?: ReactNode
}

export function Button({
  variant = 'primary',
  size = 'md',
  loading = false,
  fullWidth = false,
  square = false,
  icon,
  disabled,
  className,
  children,
  type = 'button',
  ...rest
}: ButtonProps) {
  return (
    <button
      type={type}
      disabled={disabled || loading}
      aria-busy={loading || undefined}
      className={cx(
        styles.button,
        styles[variant],
        styles[size],
        fullWidth && styles.fullWidth,
        square && styles.square,
        className,
      )}
      {...rest}
    >
      {loading ? <Spinner size={16} /> : icon}
      {children}
    </button>
  )
}
