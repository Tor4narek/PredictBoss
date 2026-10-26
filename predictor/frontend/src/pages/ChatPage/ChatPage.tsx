import { useEffect, useRef } from 'react'
import { Clock3 } from 'lucide-react'
import { useNavigate } from 'react-router'
import { useChat, useSendMessage } from '../../api/hooks'
import { formatLongDateTime } from '../../lib/format'
import { Badge, Button, Composer, ErrorState, LoadingState, Panel } from '../../ui'
import {
  AnalystMessageView,
  GreetingMessage,
  TypingMessage,
  UserMessageView,
} from './ChatMessages'
import styles from './ChatPage.module.css'

/** Диалог компании с Константином: анализ, вопросы и ответы (фреймы 05 и 08). */
export function ChatPage() {
  const chat = useChat()
  const send = useSendMessage()
  const navigate = useNavigate()
  const listEnd = useRef<HTMLDivElement>(null)

  const messages = chat.data?.messages ?? []
  const lastAnalysisAt = chat.data?.lastAnalysisAt ?? null

  // Новое сообщение — прокручиваем вниз.
  useEffect(() => {
    listEnd.current?.scrollIntoView({ block: 'end' })
  }, [messages.length, send.isPending])

  return (
    <div className={styles.page}>
      <header className={styles.header}>
        <div className={styles.headings}>
          <div className={styles.badges}>
            <Badge>Константин</Badge>
          </div>
          <h1 className={styles.title}>Диалог компании</h1>
          <p className={styles.description}>
            Ваш профиль, источники и анализ влияния событий на бизнес.
          </p>
        </div>
        {chat.isSuccess && (
          <Badge tone="accent">
            <Clock3 size={16} aria-hidden="true" />
            Последний анализ: {lastAnalysisAt ? formatLongDateTime(lastAnalysisAt) : 'нет'}
          </Badge>
        )}
      </header>

      {chat.isPending && (
        <Panel className={styles.statePanel}>
          <LoadingState label="Загружаем диалог…" />
        </Panel>
      )}

      {chat.isError && (
        <Panel className={styles.statePanel}>
          <ErrorState
            title="Не удалось загрузить диалог"
            description={chat.error.message}
            onRetry={() => chat.refetch()}
          />
        </Panel>
      )}

      {chat.isSuccess && (
        <>
          <div className={styles.messages}>
            {messages.length === 0 && (
              <GreetingMessage
                actions={
                  <>
                    <Button variant="secondary" onClick={() => navigate('/sources')}>
                      Добавить источник
                    </Button>
                    <Button variant="secondary" onClick={() => navigate('/profile')}>
                      Заполнить профиль
                    </Button>
                  </>
                }
              />
            )}
            {chat.data.processing && <TypingMessage />}
            {messages.map((message) =>
              message.role === 'user' ? (
                <UserMessageView key={message.id} message={message} />
              ) : (
                <AnalystMessageView key={message.id} message={message} />
              ),
            )}
            {send.isPending && (
              <>
                <UserMessageView
                  message={{ text: send.variables, createdAt: new Date(send.submittedAt).toISOString() }}
                />
                <TypingMessage />
              </>
            )}
            <div ref={listEnd} />
          </div>

          <div className={styles.prompt}>
            <div className={styles.secondaryActions}>
              <span className={styles.hint}>Добавить:</span>
              <Button variant="secondary" onClick={() => navigate('/sources')}>
                Источник
              </Button>
              <Button variant="secondary" onClick={() => navigate('/materials/new')}>
                Ссылку
              </Button>
              <span className={styles.spacer} />
              <span className={styles.hint}>
                Чтобы запустить анализ — напишите «Запусти анализ»
              </span>
            </div>
            {send.isError && (
              <p className={styles.sendError} role="alert">
                Сообщение не отправлено: {send.error.message}
              </p>
            )}
            <Composer
              placeholder="Введите вопрос или фразу для анализа"
              loading={send.isPending}
              onSubmit={(text) => send.mutate(text)}
            />
          </div>
        </>
      )}
    </div>
  )
}
