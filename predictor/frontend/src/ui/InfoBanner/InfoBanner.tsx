import type { ReactNode } from 'react'
import { WebIcon } from '../icons/FigmaIcons'
import { Panel } from '../Panel/Panel'
import { cx } from '../cx'
import styles from './InfoBanner.module.css'

export type InfoBannerProps = {
  title: ReactNode
  children: ReactNode
  /** Иконка 24×24 в розовом кружке. По умолчанию — «web» из макета. */
  icon?: ReactNode
  className?: string
}

/** Плашка-подсказка «Инфо»: иконка в кружке, заголовок и пояснение. */
export function InfoBanner({ title, children, icon, className }: InfoBannerProps) {
  return (
    <Panel padding="md" className={cx(styles.banner, className)}>
      <div className={styles.row}>
        <span className={styles.iconWrap}>{icon ?? <WebIcon />}</span>
        <div className={styles.text}>
          <p className={styles.title}>{title}</p>
          <p className={styles.body}>{children}</p>
        </div>
      </div>
    </Panel>
  )
}
