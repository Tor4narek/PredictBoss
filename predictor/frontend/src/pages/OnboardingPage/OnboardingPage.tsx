import { useState } from 'react'
import { useNavigate } from 'react-router'
import { useImportProfile, useProfile, useSaveProfile, useSendMessage, useSources } from '../../api/hooks'
import { Button, ErrorState, LoadingState, PageHeader, Panel, Steps } from '../../ui'
import { ProfileCreationFields, type CreationMode } from '../ProfilePage/ProfileFields'
import {
  toProfileInput,
  useProfileForm,
  validateProfile,
} from '../ProfilePage/profileForm'
import profileStyles from '../ProfilePage/ProfilePage.module.css'
import { AddSourceForm } from '../SourcesPage/AddSourceForm'
import { SourcesTable } from '../SourcesPage/SourcesTable'
import styles from './OnboardingPage.module.css'

const STEPS = ['Компания', 'Источники', 'Анализ']

/** Мастер создания компании: профиль → источники → первый анализ (фрейм «02 · Создание компании»). */
export function OnboardingPage() {
  const profile = useProfile()

  if (profile.isPending) return <LoadingState label="Загружаем профиль…" />
  if (profile.isError) {
    return (
      <ErrorState
        title="Не удалось загрузить профиль"
        description={profile.error.message}
        onRetry={() => profile.refetch()}
      />
    )
  }
  return <OnboardingWizard initialProfile={profile.data} />
}

function OnboardingWizard({ initialProfile }: { initialProfile: Parameters<typeof useProfileForm>[0] }) {
  const navigate = useNavigate()
  const [step, setStep] = useState(0)
  const form = useProfileForm(initialProfile)
  const [errors, setErrors] = useState<ReturnType<typeof validateProfile> & { syntheticU?: string }>({})
  const [mode, setMode] = useState<CreationMode>('manual')
  const [syntheticU, setSyntheticU] = useState('')
  const saveProfile = useSaveProfile()
  const importProfile = useImportProfile()
  const sources = useSources()
  const sendMessage = useSendMessage()

  const hasSources = (sources.data?.length ?? 0) > 0

  function handleBack() {
    if (step === 0) navigate(-1)
    else setStep(step - 1)
  }

  function handleNext() {
    if (step === 0) {
      if (mode === 'dataset') {
        if (!syntheticU.trim()) {
          setErrors({ syntheticU: 'Укажите идентификатор компании из набора Альфы.' })
          return
        }
        setErrors({})
        importProfile.mutate(syntheticU.trim(), { onSuccess: () => setStep(1) })
        return
      }
      const nextErrors = validateProfile(form.values)
      setErrors(nextErrors)
      if (Object.keys(nextErrors).length > 0) return
      saveProfile.mutate(toProfileInput(form.values), { onSuccess: () => setStep(1) })
      return
    }
    if (step === 1) {
      setStep(2)
      return
    }
    sendMessage.mutate(
      { text: 'Запусти анализ', requestId: crypto.randomUUID() },
      { onSuccess: () => navigate('/chat') },
    )
  }

  const nextLabel = step === 2 ? 'Запустить анализ' : 'Продолжить'
  const loading = saveProfile.isPending || importProfile.isPending || sendMessage.isPending
  const error = step === 0 ? saveProfile.error ?? importProfile.error : step === 2 ? sendMessage.error : null

  return (
    <>
      <PageHeader
        title="Расскажите о вашем бизнесе"
        description="Заполните профиль компании содержательными данными для новостной аналитики «Альфа Пульс»."
        actions={
          <>
            <Button variant="secondary" onClick={handleBack} disabled={loading}>
              Назад
            </Button>
            <Button onClick={handleNext} loading={loading}>
              {nextLabel}
            </Button>
          </>
        }
      />
      <Steps steps={STEPS} current={step} />

      {(errors.form || error) && (
        <p className={profileStyles.formError} role="alert">
          {errors.form ?? error?.message}
        </p>
      )}

      {step === 0 && (
        <ProfileCreationFields form={form} errors={errors} disabled={loading}
          mode={mode} onModeChange={(next) => { setMode(next); setErrors({}) }}
          syntheticUInput={syntheticU} onSyntheticUChange={setSyntheticU}
          syntheticUError={errors.syntheticU} />
      )}

      {step === 1 && (
        <>
          <AddSourceForm />
          <SourcesTable />
        </>
      )}

      {step === 2 && (
        <Panel
          title="Всё готово к первому анализу"
          description="Константин сопоставит новости из источников с профилем компании и покажет риски, возможности и доказательства."
        >
          <ul className={styles.checklist}>
            <li>Профиль компании заполнен.</li>
            <li>
              {hasSources
                ? `Источников добавлено: ${sources.data?.length}.`
                : 'Источников пока нет — анализ будет только по разовым материалам. Их можно добавить позже.'}
            </li>
            <li>Анализ можно перезапустить в диалоге фразой «Запусти анализ».</li>
          </ul>
        </Panel>
      )}
    </>
  )
}
