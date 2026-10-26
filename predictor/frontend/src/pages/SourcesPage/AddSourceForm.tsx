import { useState, type FormEvent } from 'react'
import { useAddSource } from '../../api/hooks'
import { Button, Field, Panel } from '../../ui'
import styles from './SourcesPage.module.css'

/** Карточка «Добавление регулярного источника». Используется на странице и в мастере создания. */
export function AddSourceForm() {
  const [url, setUrl] = useState('')
  const addSource = useAddSource()

  function handleSubmit(event: FormEvent) {
    event.preventDefault()
    addSource.mutate(url.trim(), { onSuccess: () => setUrl('') })
  }

  return (
    <Panel
      title="Добавление регулярного источника"
      description="Тип RSS/Atom/Sitemap/HTML, название и интервал определяет сервер — не пользователь. URL можно только добавить или удалить."
    >
      <form className={styles.addForm} onSubmit={handleSubmit} noValidate>
        <Field
          className={styles.addField}
          label="URL источника"
          type="url"
          inputMode="url"
          placeholder="https://example.org/news"
          value={url}
          onChange={(event) => {
            setUrl(event.target.value)
            if (addSource.isError) addSource.reset()
          }}
          error={addSource.isError ? addSource.error.message : undefined}
          required
        />
        <Button
          type="submit"
          size="lg"
          className={styles.addButton}
          loading={addSource.isPending}
        >
          Добавить источник
        </Button>
      </form>
    </Panel>
  )
}
