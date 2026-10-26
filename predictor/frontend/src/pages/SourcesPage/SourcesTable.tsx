import type { Source } from '../../api'
import { useDeleteSource, useSetSourcePaused, useSources } from '../../api/hooks'
import { formatInterval, formatShortDateTime } from '../../lib/format'
import {
  ActionLink,
  Badge,
  EmptyState,
  ErrorState,
  LoadingState,
  Panel,
  Table,
  type TableColumn,
} from '../../ui'
import { SOURCE_STATUS, SOURCE_TYPE } from './sourceLabels'
import styles from './SourcesPage.module.css'

/** Карточка «Список источников» с таблицей и состояниями загрузки, пусто, ошибка. */
export function SourcesTable() {
  const sources = useSources()
  const setPaused = useSetSourcePaused()
  const deleteSource = useDeleteSource()

  // id строки, над которой сейчас выполняется действие, — его кнопки блокируем.
  const busyId =
    (setPaused.isPending && setPaused.variables.id) ||
    (deleteSource.isPending && deleteSource.variables) ||
    null

  const columns: TableColumn<Source>[] = [
    {
      key: 'url',
      title: 'URL',
      render: (source) => <span className={styles.url}>{source.url}</span>,
    },
    {
      key: 'name',
      title: 'Имя',
      width: 152,
      render: (source) => source.name ?? source.statusReason ?? '—',
    },
    {
      key: 'type',
      title: 'Тип',
      width: 82,
      render: (source) => (source.type ? SOURCE_TYPE[source.type] : '—'),
    },
    {
      key: 'status',
      title: 'Статус',
      width: 192,
      render: (source) => (
        <Badge tone={SOURCE_STATUS[source.status].tone}>{SOURCE_STATUS[source.status].label}</Badge>
      ),
    },
    {
      key: 'lastPolledAt',
      title: 'Последний опрос',
      width: 142,
      render: (source) =>
        source.lastPolledAt
          ? formatShortDateTime(source.lastPolledAt)
          : source.status === 'checking'
            ? 'Ещё не опрашивался'
            : '—',
    },
    {
      key: 'interval',
      title: 'Интервал',
      width: 92,
      muted: true,
      render: (source) =>
        source.pollIntervalMinutes && source.status !== 'blocked' && source.status !== 'unsupported'
          ? formatInterval(source.pollIntervalMinutes)
          : '—',
    },
    {
      key: 'actions',
      title: 'Действия',
      width: 96,
      render: (source) => {
        const busy = busyId === source.id
        return (
          <div className={styles.actions}>
            {source.status === 'active' && (
              <ActionLink
                disabled={busy}
                onClick={() => setPaused.mutate({ id: source.id, paused: true })}
              >
                Пауза
              </ActionLink>
            )}
            {source.status === 'paused' && (
              <ActionLink
                disabled={busy}
                onClick={() => setPaused.mutate({ id: source.id, paused: false })}
              >
                Возобновить
              </ActionLink>
            )}
            <ActionLink
              disabled={busy}
              onClick={() => {
                if (window.confirm(`Удалить источник ${source.url}? История материалов сохранится.`)) {
                  deleteSource.mutate(source.id)
                }
              }}
            >
              Удалить
            </ActionLink>
          </div>
        )
      },
    },
  ]

  const actionError = setPaused.error ?? deleteSource.error

  return (
    <Panel
      title="Список источников"
      description="История материалов сохраняется при удалении."
    >
      {actionError && (
        <p className={styles.actionError} role="alert">
          {actionError.message}
        </p>
      )}
      {sources.isPending && <LoadingState label="Загружаем источники…" />}
      {sources.isError && (
        <ErrorState
          title="Не удалось загрузить источники"
          description={sources.error.message}
          onRetry={() => sources.refetch()}
        />
      )}
      {sources.isSuccess && sources.data.length === 0 && (
        <EmptyState
          title="Источников пока нет"
          description="Добавьте сайт или RSS-ленту выше — сервер сам определит тип и начнёт регулярный сбор новостей."
        />
      )}
      {sources.isSuccess && sources.data.length > 0 && (
        <Table columns={columns} rows={sources.data} getRowKey={(source) => source.id} />
      )}
    </Panel>
  )
}
