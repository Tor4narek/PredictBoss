import type { ReactNode } from 'react'
import type { AnalystMessage, Evidence, UserMessage } from '../../api'
import { formatLongDateTime, shortUrl } from '../../lib/format'
import { Badge, Spinner, WebIcon } from '../../ui'
import { cx } from '../../ui/cx'
import styles from './ChatMessages.module.css'

/* Сообщения диалога по фрейму «08 · Диалог компании». */

function AnalystHeader({ createdAt }: { createdAt?: string }) {
  return (
    <div className={styles.analystHeader}>
      <span className={styles.avatar} aria-hidden="true">
        К
      </span>
      <span className={styles.author}>Константин</span>
      {createdAt && <span className={styles.time}>{formatLongDateTime(createdAt)}</span>}
    </div>
  )
}

export function AnalystMessageView({ message }: { message: AnalystMessage }) {
  return (
    <article className={styles.message}>
      <AnalystHeader createdAt={message.createdAt} />
      {message.analysis ? (
        <div className={cx(styles.bubble, styles.analysis)}>
          <div className={styles.analysisHead}>
            <h3 className={styles.analysisTitle}>{message.analysis.title}</h3>
            <p className={styles.analysisSubtitle}>{message.analysis.subtitle}</p>
          </div>
          <p className={styles.text}>{message.analysis.text}</p>
          {message.analysis.evidence.length > 0 && (
            <div className={styles.evidenceBlock}>
              <p className={styles.sectionTitle}>
                Доказательство{' '}
                {message.analysis.evidence.map((item) => `[${item.index}]`).join(' ')}
              </p>
              {message.analysis.evidence.map((item) => (
                <div key={item.index} className={styles.evidenceItem}>
                  <a className={styles.evidenceLink} href={item.url} target="_blank" rel="noreferrer">
                    <WebIcon size={16} />
                    {shortUrl(item.url)}
                  </a>
                  {item.note && <p className={styles.note}>{item.note}</p>}
                </div>
              ))}
            </div>
          )}
        </div>
      ) : (
        <div className={styles.bubble}>
          {message.text && <p className={styles.text}>{message.text}</p>}
          {message.evidence.map((item) => (
            <EvidenceChip key={item.index} evidence={item} />
          ))}
          {message.note && <p className={styles.note}>{message.note}</p>}
        </div>
      )}
    </article>
  )
}

function EvidenceChip({ evidence }: { evidence: Evidence }) {
  return (
    <div className={styles.evidenceChip}>
      <span className={styles.evidenceIndex}>Источник [{evidence.index}]</span>
      <span className={styles.evidenceTitle}>{evidence.title}</span>
      <a className={styles.evidenceUrl} href={evidence.url} target="_blank" rel="noreferrer">
        {shortUrl(evidence.url)}
      </a>
    </div>
  )
}

export function UserMessageView({ message }: { message: Pick<UserMessage, 'text' | 'createdAt'> }) {
  return (
    <article className={cx(styles.message, styles.userMessage)}>
      <div className={styles.userHeader}>
        <span className={styles.time}>{formatLongDateTime(message.createdAt)}</span>
        <Badge>Пользователь</Badge>
      </div>
      <div className={styles.userBubble}>
        <p className={styles.text}>{message.text}</p>
      </div>
    </article>
  )
}

/** Константин готовит ответ. */
export function TypingMessage() {
  return (
    <article className={styles.message} aria-live="polite">
      <AnalystHeader />
      <div className={cx(styles.bubble, styles.typing)}>
        <Spinner size={16} />
        <span>Константин готовит ответ…</span>
      </div>
    </article>
  )
}

/** Приветствие в пустом диалоге (фрейм «05 · Анализ — начало», оформлено как сообщения из «08»). */
export function GreetingMessage({ actions }: { actions: ReactNode }) {
  return (
    <article className={styles.message}>
      <AnalystHeader />
      <div className={cx(styles.bubble, styles.greeting)}>
        <p className={styles.greetingTitle}>Привет! Я Константин, ваш аналитик.</p>
        <p className={styles.greetingText}>
          Помогу разобраться, как новости влияют на ваш бизнес: объясню риски, возможности и покажу
          источники.
        </p>
        <p className={styles.greetingText}>
          Для анализа заполните профиль компании и добавьте хотя бы один источник. Затем просто
          напишите «Запусти анализ». А задать мне вопрос можно уже сейчас.
        </p>
        <div className={styles.greetingActions}>{actions}</div>
      </div>
    </article>
  )
}
