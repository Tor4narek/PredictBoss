import { useState, type FormEvent } from 'react'
import { useNavigate } from 'react-router'
import type { CompanyProfile } from '../../api'
import { useImportProfile, useProfile, useSaveProfile } from '../../api/hooks'
import { Button, ErrorState, LoadingState, PageHeader, Panel } from '../../ui'
import { ProfileCreationFields, ProfileSectionFields, type CreationMode } from './ProfileFields'
import { toProfileInput, useProfileForm, validateProfile } from './profileForm'
import styles from './ProfilePage.module.css'

/**
 * Создание из CSV или вручную; редактирование всех полей профиля по разделам.
 */
export function ProfilePage() {
  const profile = useProfile()

  if (profile.isPending) {
    return (
      <>
        <PageHeader title="Профиль компании" />
        <Panel>
          <LoadingState label="Загружаем профиль…" />
        </Panel>
      </>
    )
  }

  if (profile.isError) {
    return (
      <>
        <PageHeader title="Профиль компании" />
        <Panel>
          <ErrorState
            title="Не удалось загрузить профиль"
            description={profile.error.message}
            onRetry={() => profile.refetch()}
          />
        </Panel>
      </>
    )
  }

  return profile.data ? <EditProfile profile={profile.data} /> : <CreateProfile />
}

function CreateProfile() {
  const navigate = useNavigate()
  const form = useProfileForm(null)
  const save = useSaveProfile()
  const importProfile = useImportProfile()
  const [mode, setMode] = useState<CreationMode>('manual')
  const [syntheticU, setSyntheticU] = useState('')
  const [errors, setErrors] = useState<ReturnType<typeof validateProfile> & { syntheticU?: string }>({})
  const loading = save.isPending || importProfile.isPending

  function handleSubmit(event: FormEvent) {
    event.preventDefault()
    if (mode === 'dataset') {
      if (!syntheticU.trim()) {
        setErrors({ syntheticU: 'Укажите идентификатор компании из набора Альфы.' })
        return
      }
      setErrors({})
      importProfile.mutate(syntheticU.trim())
      return
    }
    const nextErrors = validateProfile(form.values)
    setErrors(nextErrors)
    if (Object.keys(nextErrors).length > 0) return
    save.mutate(toProfileInput(form.values))
  }

  return (
    <form className={styles.page} onSubmit={handleSubmit} noValidate>
      <PageHeader
        title="Профиль компании"
        description="Выберите компанию из набора Альфы или заполните её данные вручную."
        actions={
          <>
            <Button variant="secondary" onClick={() => navigate(-1)}>
              Назад
            </Button>
            <Button type="submit" loading={loading}>
              Сохранить профиль
            </Button>
          </>
        }
      />
      <FormMessages formError={errors.form} saveError={(save.error ?? importProfile.error)?.message} />
      <Panel>
        <ProfileCreationFields form={form} errors={errors} disabled={loading}
          mode={mode} onModeChange={(next) => { setMode(next); setErrors({}) }}
          syntheticUInput={syntheticU} onSyntheticUChange={setSyntheticU}
          syntheticUError={errors.syntheticU} />
      </Panel>
    </form>
  )
}

function EditProfile({ profile }: { profile: CompanyProfile }) {
  const form = useProfileForm(profile)
  const save = useSaveProfile()
  const [errors, setErrors] = useState<ReturnType<typeof validateProfile>>({})
  const [saved, setSaved] = useState(false)

  function handleSubmit(event: FormEvent) {
    event.preventDefault()
    const nextErrors = validateProfile(form.values)
    setErrors(nextErrors)
    if (Object.keys(nextErrors).length > 0) return
    save.mutate(toProfileInput(form.values), {
      onSuccess: (next) => {
        form.reset(next)
        setSaved(true)
      },
    })
  }

  function handleCancel() {
    form.reset()
    setErrors({})
    save.reset()
  }

  const actions = (
    <>
      <Button variant="secondary" onClick={handleCancel} disabled={save.isPending}>
        Отменить
      </Button>
      <Button type="submit" loading={save.isPending}>
        Сохранить изменения
      </Button>
    </>
  )

  return (
    <form
      className={styles.page}
      onSubmit={handleSubmit}
      onChange={() => setSaved(false)}
      noValidate
    >
      <PageHeader
        title="Профиль компании"
        description="Все данные профиля компании, используемые для новостной аналитики."
        actions={actions}
      />
      <FormMessages formError={errors.form} saveError={save.error?.message} />
      <Panel>
        <ProfileSectionFields
          form={form}
          errors={errors}
          disabled={save.isPending}
          syntheticU={profile.syntheticU}
          footer={
            <div className={styles.footer}>
              <p className={saved ? styles.success : styles.footerNote} role="status">
                {saved
                  ? 'Профиль сохранён. Анализ обновится с учётом новых данных.'
                  : 'Изменения обновят контекст, затем существующий анализ.'}
              </p>
              <div className={styles.footerActions}>{actions}</div>
            </div>
          }
        />
      </Panel>
    </form>
  )
}

function FormMessages({ formError, saveError }: { formError?: string; saveError?: string }) {
  if (!formError && !saveError) return null
  return (
    <p className={styles.formError} role="alert">
      {formError ?? `Не удалось сохранить профиль: ${saveError}`}
    </p>
  )
}
