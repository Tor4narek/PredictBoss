import { useState, type FormEvent } from 'react'
import { useNavigate } from 'react-router'
import { useAddMaterial } from '../../api/hooks'
import { Button, Field, PageHeader, Panel } from '../../ui'
import styles from './MaterialNewPage.module.css'

/** Разовая ссылка на новость или анализ (фрейм «12 · Добавление разового материала»). */
export function MaterialNewPage() {
  const navigate = useNavigate()
  const [url, setUrl] = useState('')
  const addMaterial = useAddMaterial()

  function handleSubmit(event: FormEvent) {
    event.preventDefault()
    addMaterial.mutate(url.trim(), { onSuccess: () => navigate('/materials') })
  }

  return (
    <>
      <PageHeader
        title="Добавление новости"
        description="Добавьте ссылку на новость или аналитический материал банка."
      />
      <Panel
        padding="xl"
        title="Новый материал"
        description="Обработаем один раз, без расписания. Для постоянного сбора добавьте источник."
      >
        <form className={styles.form} onSubmit={handleSubmit} noValidate>
          <Field
            label="HTTPS URL"
            type="url"
            inputMode="url"
            placeholder="https://alfabank.ru/example/analysis.pdf"
            hint="Пример URL: https://alfabank.ru/example/analysis.pdf"
            value={url}
            onChange={(event) => {
              setUrl(event.target.value)
              if (addMaterial.isError) addMaterial.reset()
            }}
            error={addMaterial.isError ? addMaterial.error.message : undefined}
            required
          />
          <div className={styles.footer}>
            <Button variant="ghost" size="lg" className={styles.cancel} onClick={() => navigate(-1)}>
              Отмена
            </Button>
            <Button
              type="submit"
              size="lg"
              className={styles.submit}
              loading={addMaterial.isPending}
            >
              Добавить материал
            </Button>
          </div>
        </form>
      </Panel>
    </>
  )
}
