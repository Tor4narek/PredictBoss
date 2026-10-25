import { Badge } from '../Badge/Badge'
import { cx } from '../cx'
import styles from './Steps.module.css'

export type StepsProps = {
  /** Названия шагов по порядку. */
  steps: string[]
  /** Номер текущего шага, начиная с 0. */
  current: number
  className?: string
}

/** Шаги мастера: «1 Компания · 2 Источники · 3 Анализ». Текущий шаг выделен тёмным текстом. */
export function Steps({ steps, current, className }: StepsProps) {
  return (
    <ol className={cx(styles.steps, className)}>
      {steps.map((label, index) => {
        const isCurrent = index === current
        return (
          <li
            key={label}
            className={cx(styles.step, isCurrent && styles.current)}
            aria-current={isCurrent ? 'step' : undefined}
          >
            <Badge round>{index + 1}</Badge>
            <span>{label}</span>
          </li>
        )
      })}
    </ol>
  )
}
