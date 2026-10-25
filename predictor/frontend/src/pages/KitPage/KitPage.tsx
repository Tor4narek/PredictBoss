import type { ReactNode } from 'react'
import {
  ActionLink,
  AutorenewIcon,
  Badge,
  Button,
  CheckSmallIcon,
  Chip,
  Composer,
  DEFAULT_SIDEBAR_ITEMS,
  EmptyState,
  ErrorState,
  Field,
  InfoBanner,
  Input,
  LoadingState,
  NavItem,
  PageHeader,
  Panel,
  Sidebar,
  Steps,
  Table,
  WebIcon,
  type TableColumn,
} from '../../ui'
import styles from './KitPage.module.css'

/** Витрина UI kit: все компоненты и их состояния на одной странице, чтобы сверить с Figma. */
export function KitPage() {
  return (
    <main className={styles.page}>
      <header className={styles.intro}>
        <h1 className={styles.pageTitle}>Альфа Пульс / UI Kit</h1>
        <p className={styles.pageSubtitle}>
          Локальные компоненты · Inter · единые цвета, размеры и состояния. Сравните с фреймом
          «UI Kit · Альфа Пульс» в Figma.
        </p>
      </header>

      <div className={styles.grid}>
        <Section title="01 / Кнопки" note="Основная · вторичная · текстовая · недоступная. Высота 40 / 48 · скругление 8 · Inter 14">
          <div className={styles.column}>
            {(['primary', 'secondary', 'ghost'] as const).map((variant) => (
              <div key={variant} className={styles.row}>
                <Button variant={variant}>Кнопка</Button>
                <Button variant={variant} size="lg">
                  Кнопка
                </Button>
                <Button variant={variant} loading>
                  Кнопка
                </Button>
              </div>
            ))}
            <div className={styles.row}>
              <Button disabled>Кнопка</Button>
              <Button disabled size="lg">
                Кнопка
              </Button>
            </div>
          </div>
        </Section>

        <Section title="02 / Поля">
          <div className={styles.stack}>
            <Input defaultValue="Нет данных" />
            <Input defaultValue="Нет данных" disabled />
            <Input placeholder="Плейсхолдер" />
            <Input defaultValue="Ошибка" invalid />
          </div>
        </Section>

        <Section title="Поле с подписью">
          <div className={styles.stack}>
            <Field label="Название поля" defaultValue="Нет данных" />
            <Field label="HTTPS URL" placeholder="https://example.org/news" hint="Пример URL" />
            <Field label="HTTPS URL" defaultValue="http://example.org" error="Нужен адрес, начинающийся с https://" />
            <Field label="Недоступно" defaultValue="Нет данных" disabled />
          </div>
        </Section>

        <Section title="03 / Навигация">
          <div className={styles.column}>
            {DEFAULT_SIDEBAR_ITEMS.map((item) => (
              <div key={item.to} className={styles.column}>
                <NavItem {...item} active={false} />
                <NavItem {...item} active />
              </div>
            ))}
          </div>
        </Section>

        <Section title="04 / Статусы">
          <div className={styles.column}>
            <div className={styles.row}>
              <Badge>Статус</Badge>
              <Badge tone="accent">Статус</Badge>
              <Badge tone="success">Статус</Badge>
              <Badge tone="warning">Статус</Badge>
            </div>
            <div className={styles.row}>
              <Badge tone="success">Активен</Badge>
              <Badge tone="accent">Проверяется</Badge>
              <Badge>На паузе</Badge>
              <Badge tone="warning">Повторная попытка</Badge>
            </div>
            <div className={styles.row}>
              <Badge round>1</Badge>
              <Badge round>2</Badge>
              <Badge round>3</Badge>
            </div>
          </div>
        </Section>

        <Section title="Действие в таблице">
          <div className={styles.column}>
            <ActionLink href="https://example.org">Подробнее ↗</ActionLink>
            <ActionLink>Пауза</ActionLink>
            <ActionLink>Удалить</ActionLink>
            <ActionLink disabled>Недоступно</ActionLink>
          </div>
        </Section>

        <Section title="Чипы">
          <div className={styles.row}>
            <Chip icon={<WebIcon />}>HTTPS только</Chip>
            <Chip icon={<AutorenewIcon />}>Автопроверка</Chip>
            <Chip icon={<CheckSmallIcon />}>Без авторизации</Chip>
          </div>
        </Section>

        <Section title="Шаги мастера">
          <div className={styles.column}>
            <Steps steps={['Компания', 'Источники', 'Анализ']} current={0} />
            <Steps steps={['Компания', 'Источники', 'Анализ']} current={1} />
          </div>
        </Section>
      </div>

      <Section title="05 / Карточка" note="Общий контейнер со слотом Content. Содержимое карточек сохраняется.">
        <div className={styles.cards}>
          <Panel className={styles.emptyPanel} />
          <Panel
            title="Список источников"
            description="Демонстрационные данные. История материалов сохраняется при удалении."
            aside={<Badge tone="accent">Демонстрационные данные</Badge>}
          >
            <p className={styles.muted}>Содержимое карточки.</p>
          </Panel>
          <InfoBanner title="Инфо">
            Разовая статья? Добавьте ссылку в разделе «Материалы». Публичный HTTPS, без авторизации и
            CAPTCHA.
          </InfoBanner>
        </div>
      </Section>

      <Section title="06 / Поле сообщения" note="Готово к вопросу / до первого анализа / отправляется">
        <div className={styles.composers}>
          <Composer onSubmit={(text) => alert(`Отправлено: ${text}`)} />
          <Composer onSubmit={() => {}} disabled />
          <Composer onSubmit={() => {}} loading />
        </div>
      </Section>

      <Section title="Шапка страницы">
        <div className={styles.pageHeaderDemo}>
          <PageHeader
            title="Источники новостей"
            description="Регулярный сбор публикаций из добавленных сайтов и лент."
            actions={
              <>
                <Chip icon={<WebIcon />}>HTTPS только</Chip>
                <Chip icon={<AutorenewIcon />}>Автопроверка</Chip>
              </>
            }
          />
          <PageHeader
            title="Профиль компании"
            description="Пример заполнения профиля — показаны только поля с точными значениями."
            actions={
              <>
                <Button variant="secondary">Отменить</Button>
                <Button>Сохранить изменения</Button>
              </>
            }
          />
        </div>
      </Section>

      <Section title="Таблица">
        <Panel title="Список источников" aside={<Badge tone="accent">Демонстрационные данные</Badge>}>
          <Table columns={demoColumns} rows={demoRows} getRowKey={(row) => row.url} />
        </Panel>
      </Section>

      <Section title="Состояния: загрузка, пусто, ошибка" note="В Figma этих состояний нет — собраны из токенов.">
        <div className={styles.cards}>
          <Panel>
            <LoadingState />
          </Panel>
          <Panel>
            <EmptyState
              title="Источников пока нет"
              description="Добавьте сайт или RSS-ленту, чтобы начать сбор новостей."
              action={<Button>Добавить источник</Button>}
            />
          </Panel>
          <Panel>
            <ErrorState onRetry={() => {}} />
          </Panel>
        </div>
      </Section>

      <Section title="07 / Боковая панель">
        <div className={styles.sidebars}>
          {DEFAULT_SIDEBAR_ITEMS.map((item) => (
            <Sidebar key={item.to} activeTo={item.to} className={styles.sidebarDemo} />
          ))}
        </div>
      </Section>

      <Section title="08 / Цветовые роли">
        <div className={styles.swatches}>
          {COLORS.map((name) => (
            <div key={name} className={styles.swatch}>
              <span className={styles.swatchColor} style={{ background: `var(--color-${name})` }} />
              <span className={styles.swatchName}>{name}</span>
            </div>
          ))}
        </div>
      </Section>

      <Section title="09 / Типографика">
        <div className={styles.column}>
          <p className={styles.headingL}>Heading / L — Новости в контексте бизнеса</p>
          <p className={styles.headingM}>Heading / M — Новости в контексте бизнеса</p>
          <p className={styles.bodyM}>Body / M — Новости в контексте бизнеса</p>
          <p className={styles.labelM}>Label / M — Новости в контексте бизнеса</p>
          <p className={styles.caption}>Caption — Новости в контексте бизнеса</p>
        </div>
      </Section>
    </main>
  )
}

function Section({ title, note, children }: { title: string; note?: string; children: ReactNode }) {
  return (
    <section className={styles.section}>
      <h2 className={styles.sectionTitle}>{title}</h2>
      {children}
      {note && <p className={styles.note}>{note}</p>}
    </section>
  )
}

const COLORS = [
  'accent',
  'accent-subtle',
  'text-primary',
  'text-secondary',
  'canvas',
  'surface',
  'field',
  'border',
  'success',
  'success-subtle',
  'warning',
  'warning-subtle',
]

type DemoRow = { url: string; name: string; type: string; status: ReactNode; interval: string }

const demoRows: DemoRow[] = [
  { url: 'https://example.org/news', name: 'Example News', type: 'HTML', status: <Badge tone="success">Активен</Badge>, interval: '15 мин' },
  { url: 'https://example.net/feed', name: 'Example Feed', type: 'RSS', status: <Badge tone="accent">Проверяется</Badge>, interval: '15 мин' },
  { url: 'https://example.io/sitemap.xml', name: 'Example Sitemap', type: 'Sitemap', status: <Badge>На паузе</Badge>, interval: '1 час' },
]

const demoColumns: TableColumn<DemoRow>[] = [
  { key: 'url', title: 'URL', render: (row) => row.url },
  { key: 'name', title: 'Имя', width: 152, render: (row) => row.name },
  { key: 'type', title: 'Тип', width: 82, render: (row) => row.type },
  { key: 'status', title: 'Статус', width: 192, render: (row) => row.status },
  { key: 'interval', title: 'Интервал', width: 92, muted: true, render: (row) => row.interval },
  {
    key: 'actions',
    title: 'Действия',
    width: 120,
    render: () => (
      <div className={styles.actions}>
        <ActionLink>Пауза</ActionLink>
        <ActionLink>Удалить</ActionLink>
      </div>
    ),
  },
]
