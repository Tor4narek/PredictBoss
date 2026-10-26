import {
  AutorenewIcon,
  CheckSmallIcon,
  Chip,
  InfoBanner,
  PageHeader,
  WebIcon,
} from '../../ui'
import { AddSourceForm } from './AddSourceForm'
import { SourcesTable } from './SourcesTable'

/** Регулярные источники новостей (фрейм «10 · Источники новостей»). */
export function SourcesPage() {
  return (
    <>
      <PageHeader
        title="Источники новостей"
        description="Регулярный сбор публикаций из добавленных сайтов и лент."
        actions={
          <>
            <Chip icon={<WebIcon />}>HTTP / HTTPS</Chip>
            <Chip icon={<AutorenewIcon />}>Автопроверка</Chip>
            <Chip icon={<CheckSmallIcon />}>Без авторизации</Chip>
          </>
        }
      />
      <InfoBanner title="Инфо">
        Разовая статья? Добавьте ссылку в разделе «Материалы». Публичные HTTP- или HTTPS-страницы без авторизации и
        CAPTCHA.
      </InfoBanner>
      <AddSourceForm />
      <SourcesTable />
    </>
  )
}
