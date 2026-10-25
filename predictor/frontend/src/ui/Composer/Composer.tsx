import { useState, type FormEvent } from 'react'
import { Button } from '../Button/Button'
import { cx } from '../cx'
import styles from './Composer.module.css'

export type ComposerProps = {
  /** Вызывается с текстом сообщения. После отправки поле очищается. */
  onSubmit: (text: string) => void
  placeholder?: string
  /** «До первого анализа»: поле и кнопка недоступны. */
  disabled?: boolean
  /** Сообщение отправляется: на кнопке спиннер. */
  loading?: boolean
  className?: string
}

/** Поле сообщения с кнопкой отправки «↑». Enter — отправить. */
export function Composer({
  onSubmit,
  placeholder = 'Введите вопрос по анализу',
  disabled = false,
  loading = false,
  className,
}: ComposerProps) {
  const [text, setText] = useState('')
  const canSend = !disabled && !loading && text.trim().length > 0

  function handleSubmit(event: FormEvent) {
    event.preventDefault()
    if (!canSend) return
    onSubmit(text.trim())
    setText('')
  }

  return (
    <form
      className={cx(styles.composer, disabled && styles.disabled, className)}
      onSubmit={handleSubmit}
    >
      <input
        className={styles.input}
        value={text}
        onChange={(event) => setText(event.target.value)}
        placeholder={placeholder}
        disabled={disabled}
        aria-label={placeholder}
      />
      <Button type="submit" square disabled={disabled} loading={loading} aria-label="Отправить">
        {loading ? null : '↑'}
      </Button>
    </form>
  )
}
