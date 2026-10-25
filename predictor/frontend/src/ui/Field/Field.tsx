import { useId, type ReactNode } from 'react'
import { cx } from '../cx'
import { Input, type InputProps } from '../Input/Input'
import styles from './Field.module.css'

export type FieldProps = InputProps & {
  /** Подпись над полем. */
  label: ReactNode
  /** Серая подсказка под полем, например «Пример URL». */
  hint?: ReactNode
  /** Текст ошибки. Если задан, поле подсвечивается красным, а подсказка скрывается. */
  error?: ReactNode
  /** Класс для обёртки (ширина, отступы). */
  className?: string
}

/** Поле с подписью: подпись, Input и подсказка или ошибка. */
export function Field({ label, hint, error, className, id, ...inputProps }: FieldProps) {
  const autoId = useId()
  const inputId = id ?? autoId
  const noteId = `${inputId}-note`
  const note = error ?? hint

  return (
    <div className={cx(styles.field, className)}>
      <label className={styles.label} htmlFor={inputId}>
        {label}
      </label>
      <Input
        id={inputId}
        invalid={Boolean(error)}
        aria-describedby={note ? noteId : undefined}
        {...inputProps}
      />
      {note && (
        <p id={noteId} className={cx(styles.note, error ? styles.error : undefined)}>
          {note}
        </p>
      )}
    </div>
  )
}
