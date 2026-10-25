import { useState } from 'react'
import { useNavigate } from 'react-router'
import { isDemoData, type Material } from '../../api'
import { useMaterials } from '../../api/hooks'
import { formatDateTime, shortUrl, toDateInputValue } from '../../lib/format'
import {
  ActionLink,
  Badge,
  Button,
  CheckSmallIcon,
  EmptyState,
  ErrorState,
  Field,
  LoadingState,
  PageHeader,
  Panel,
  Table,
  type TableColumn,
} from '../../ui'
import { MATERIAL_KIND, MATERIAL_ORIGIN, MATERIAL_STATUS } from './materialLabels'
import styles from './MaterialsPage.module.css'

const PAGE_SIZE = 5

const columns: TableColumn<Material>[] = [
  {
    key: 'title',
    title: 'Заголовок',
    render: (material) => (
      <div className={styles.titleCell}>
        <span className={styles.title} title={material.title}>
          {material.title}
        </span>
        <span className={styles.url}>{shortUrl(material.url)}</span>
      </div>
    ),
  },
  {
    key: 'publishedAt',
    title: 'Время публикации',
    width: 136,
    render: (material) => (material.publishedAt ? formatDateTime(material.publishedAt) : 'Нет данных'),
  },
  { key: 'kind', title: 'Тип', width: 176, render: (material) => MATERIAL_KIND[material.kind] },
  {
    key: 'format',
    title: 'Формат',
    width: 116,
    render: (material) => <Badge>{material.format.toUpperCase()}</Badge>,
  },
  {
    key: 'origin',
    title: 'Источник',
    width: 176,
    render: (material) => MATERIAL_ORIGIN[material.origin],
  },
  {
    key: 'status',
    title: 'Статус',
    width: 172,
    render: (material) => (
      <Badge tone={MATERIAL_STATUS[material.status].tone}>
        {MATERIAL_STATUS[material.status].label}
      </Badge>
    ),
  },
  {
    key: 'actions',
    title: 'Действия',
    width: 120,
    render: (material) => <ActionLink href={material.url}>Подробнее ↗</ActionLink>,
  },
]

function startOfMonth(): string {
  const today = toDateInputValue(new Date())
  return `${today.slice(0, 8)}01`
}

/** Материалы: новости и анализы банка по периоду (фрейм «11 · Материалы»). */
export function MaterialsPage() {
  const navigate = useNavigate()
  const [from, setFrom] = useState(startOfMonth)
  const [to, setTo] = useState(() => toDateInputValue(new Date()))
  const [limit, setLimit] = useState(PAGE_SIZE)

  const periodError = from && to && from > to ? 'Дата «От» позже даты «До»' : undefined
  const materials = useMaterials({ from, to, limit })
  const showTable = !periodError

  return (
    <>
      <PageHeader
        title="Материалы"
        description="Метаданные новостей и анализов банка, без полного текста"
        actions={
          <>
            {isDemoData && <Badge tone="accent">Демонстрационные данные</Badge>}
            <Button className={styles.addButton} onClick={() => navigate('/materials/new')}>
              Добавить разовую ссылку
            </Button>
          </>
        }
      />

      <div className={styles.filters}>
        <Field
          className={styles.dateField}
          label="От"
          type="date"
          value={from}
          max={to}
          onChange={(event) => {
            setFrom(event.target.value)
            setLimit(PAGE_SIZE)
          }}
          error={periodError}
        />
        <Field
          className={styles.dateField}
          label="До"
          type="date"
          value={to}
          min={from}
          onChange={(event) => {
            setTo(event.target.value)
            setLimit(PAGE_SIZE)
          }}
        />
        <span className={styles.spacer} />
        {materials.data?.hasMore && showTable && (
          <ActionLink
            className={styles.more}
            disabled={materials.isFetching}
            onClick={() => setLimit((value) => value + PAGE_SIZE)}
          >
            {materials.isFetching ? 'Загружаем…' : 'Показать ещё'}
          </ActionLink>
        )}
      </div>

      <Panel padding="none" className={styles.tablePanel}>
        {!showTable && (
          <EmptyState title="Неверный период" description="Выберите дату «От» не позже даты «До»." />
        )}
        {showTable && materials.isPending && <LoadingState label="Загружаем материалы…" />}
        {showTable && materials.isError && (
          <ErrorState
            title="Не удалось загрузить материалы"
            description={materials.error.message}
            onRetry={() => materials.refetch()}
          />
        )}
        {showTable && materials.isSuccess && materials.data.items.length === 0 && (
          <EmptyState
            title="За этот период материалов нет"
            description="Измените период или добавьте разовую ссылку на новость."
            action={
              <Button variant="secondary" onClick={() => navigate('/materials/new')}>
                Добавить разовую ссылку
              </Button>
            }
          />
        )}
        {showTable && materials.isSuccess && materials.data.items.length > 0 && (
          <Table
            variant="boxed"
            columns={columns}
            rows={materials.data.items}
            getRowKey={(material) => material.id}
          />
        )}
      </Panel>

      <p className={styles.footer}>
        <CheckSmallIcon />
        Статус «Принято» означает успешную доставку в ESB, но не завершение анализа.
      </p>
    </>
  )
}
