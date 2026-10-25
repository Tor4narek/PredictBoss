import type { ReactNode } from 'react'
import { cx } from '../cx'
import styles from './Table.module.css'

export type TableColumn<Row> = {
  /** Уникальный ключ колонки. */
  key: string
  /** Заголовок колонки. */
  title: ReactNode
  /** Ширина в пикселях. Колонки без ширины делят оставшееся место. */
  width?: number
  /** Серый текст в ячейках (как «Интервал» в макете). */
  muted?: boolean
  /** Как нарисовать ячейку для строки. */
  render: (row: Row) => ReactNode
}

export type TableProps<Row> = {
  columns: TableColumn<Row>[]
  rows: Row[]
  /** Уникальный ключ строки. */
  getRowKey: (row: Row) => string
  className?: string
}

/** Таблица из макета: серые заголовки 12, строки 13 с разделителями. Состояния загрузки/пусто/ошибка — снаружи. */
export function Table<Row>({ columns, rows, getRowKey, className }: TableProps<Row>) {
  return (
    <table className={cx(styles.table, className)}>
      <colgroup>
        {columns.map((column) => (
          <col key={column.key} style={column.width ? { width: column.width } : undefined} />
        ))}
      </colgroup>
      <thead>
        <tr>
          {columns.map((column) => (
            <th key={column.key} className={styles.th} scope="col">
              {column.title}
            </th>
          ))}
        </tr>
      </thead>
      <tbody>
        {rows.map((row) => (
          <tr key={getRowKey(row)} className={styles.tr}>
            {columns.map((column) => (
              <td key={column.key} className={cx(styles.td, column.muted && styles.muted)}>
                {column.render(row)}
              </td>
            ))}
          </tr>
        ))}
      </tbody>
    </table>
  )
}
