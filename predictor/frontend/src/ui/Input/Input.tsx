import type { InputHTMLAttributes } from 'react'
import { cx } from '../cx'
import styles from './Input.module.css'

export type InputProps = InputHTMLAttributes<HTMLInputElement> & {
  /** Красная рамка — значение с ошибкой. */
  invalid?: boolean
}

/** Однострочное поле ввода высотой 48. Disabled и ошибка — через стандартные атрибуты. */
export function Input({ invalid = false, className, ...rest }: InputProps) {
  return (
    <input
      aria-invalid={invalid || undefined}
      className={cx(styles.input, invalid && styles.invalid, className)}
      {...rest}
    />
  )
}
