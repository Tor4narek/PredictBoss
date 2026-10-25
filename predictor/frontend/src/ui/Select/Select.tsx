import { useId, type ReactNode, type SelectHTMLAttributes } from 'react'
import { ChevronDown } from 'lucide-react'
import { cx } from '../cx'
import fieldStyles from '../Field/Field.module.css'
import inputStyles from '../Input/Input.module.css'
import styles from './Select.module.css'

export type SelectOption = { value: string; label: string }

export type SelectProps = SelectHTMLAttributes<HTMLSelectElement> & {
  /** Подпись над полем. */
  label: ReactNode
  options: SelectOption[]
  /** Текст ошибки под полем. */
  error?: ReactNode
  /** Класс для обёртки. */
  className?: string
}

/** Выпадающий список с подписью. Выглядит как Field: те же размеры, цвета и состояния. */
export function Select({ label, options, error, className, id, ...rest }: SelectProps) {
  const autoId = useId()
  const selectId = id ?? autoId

  return (
    <div className={cx(fieldStyles.field, className)}>
      <label className={fieldStyles.label} htmlFor={selectId}>
        {label}
      </label>
      <div className={styles.wrap}>
        <select
          id={selectId}
          aria-invalid={error ? true : undefined}
          className={cx(inputStyles.input, styles.select, error ? inputStyles.invalid : undefined)}
          {...rest}
        >
          {options.map((option) => (
            <option key={option.value} value={option.value}>
              {option.label}
            </option>
          ))}
        </select>
        <ChevronDown size={16} className={styles.chevron} aria-hidden="true" />
      </div>
      {error && <p className={cx(fieldStyles.note, fieldStyles.error)}>{error}</p>}
    </div>
  )
}
