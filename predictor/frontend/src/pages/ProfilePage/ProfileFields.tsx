import type { ReactNode } from 'react'
import { Badge, Field, Select, type SelectOption } from '../../ui'
import {
  BusinessDevelopmentStage, CompanyAgeGradation, EmployeeCountGradation,
  OwnershipForm, ownershipFormLabel,
} from '../../api/companyEnums'
import {
  CLIENT_TYPE_OPTIONS, YES_NO_OPTIONS,
  type ProfileForm, type ProfileFormValues, type TriState,
} from './profileForm'
import styles from './ProfilePage.module.css'

type TextKey = 'mainOkvedCode' | 'mainOkvedName' | 'ftsCountryName' | 'groupId' | 'city' | 'region'
type EnumKey = 'ownershipForm' | 'companyAgeGradation' | 'developmentStage' |
  'workersFnsGradation' | 'workersSparkGradation'
type BooleanKey = 'negativeMark' | 'hasRko' | 'hasGuarantees' | 'hasLoans' |
  'hasLeasing' | 'hasDeposits' | 'hasPayroll' | 'ftsFlag' | 'hasFactoring'
type TextDefinition = { key: TextKey; label: string; placeholder?: string }
type EnumDefinition = { key: EnumKey; label: string; options: SelectOption[] }
type BooleanDefinition = { key: BooleanKey; label: string }

type FieldsProps = {
  form: ProfileForm
  errors: { mainOkvedCode?: string; revenue?: string }
  disabled?: boolean
  syntheticU?: string | null
}

export type CreationMode = 'manual' | 'dataset'
type CreationProps = FieldsProps & {
  mode: CreationMode
  onModeChange: (mode: CreationMode) => void
  syntheticUInput: string
  onSyntheticUChange: (value: string) => void
  syntheticUError?: string
}

const employeeCountOptions: SelectOption[] = [
  { value: EmployeeCountGradation.Under15, label: 'До 15 человек' },
  { value: EmployeeCountGradation.From16To100, label: 'От 16 до 100 человек' },
  { value: EmployeeCountGradation.From101To250, label: 'От 101 до 250 человек' },
  { value: EmployeeCountGradation.Over251, label: 'Более 251 человека' },
]

const enumFields: EnumDefinition[] = [
  { key: 'ownershipForm', label: 'Форма собственности', options: Object.values(OwnershipForm).map((value) => ({
    value, label: ownershipFormLabel(value),
  })) },
  { key: 'companyAgeGradation', label: 'Возраст компании', options: [
    { value: CompanyAgeGradation.UpTo3Years, label: 'До 3 лет' },
    { value: CompanyAgeGradation.From4To7Years, label: 'От 4 до 7 лет' },
    { value: CompanyAgeGradation.From8To15Years, label: 'От 8 до 15 лет' },
    { value: CompanyAgeGradation.From16To25Years, label: 'От 16 до 25 лет' },
  ] },
  { key: 'developmentStage', label: 'Этап развития бизнеса', options: [
    { value: BusinessDevelopmentStage.Birth, label: 'Начало деятельности' },
    { value: BusinessDevelopmentStage.RapidGrowth, label: 'Быстрый рост' },
    { value: BusinessDevelopmentStage.Optimization, label: 'Оптимизация бизнеса' },
    { value: BusinessDevelopmentStage.NewReality, label: 'Работа в новых условиях' },
  ] },
  { key: 'workersFnsGradation', label: 'Количество работников по данным ФНС', options: employeeCountOptions },
  { key: 'workersSparkGradation', label: 'Количество работников по данным СПАРК', options: employeeCountOptions },
]

const basicFields: TextDefinition[] = [
  { key: 'mainOkvedCode', label: 'Основной вид деятельности — код ОКВЭД', placeholder: '46.51' },
  { key: 'mainOkvedName', label: 'Основной вид деятельности', placeholder: 'Оптовая торговля компьютерами' },
]
const scaleFields: TextDefinition[] = [
  { key: 'groupId', label: 'Идентификатор группы компаний', placeholder: 'Если известен' },
]
const productFields: BooleanDefinition[] = [
  { key: 'hasRko', label: 'Расчётно-кассовое обслуживание' },
  { key: 'hasGuarantees', label: 'Банковские гарантии' },
  { key: 'hasLoans', label: 'Кредитные продукты' },
  { key: 'hasLeasing', label: 'Лизинговые продукты' },
  { key: 'hasDeposits', label: 'Депозитные продукты' },
  { key: 'hasPayroll', label: 'Зарплатный проект' },
  { key: 'hasFactoring', label: 'Факторинговый продукт' },
]

function TextFields({ definitions, form, errors, disabled }: FieldsProps & { definitions: TextDefinition[] }) {
  return definitions.map(({ key, label, placeholder }) => (
    <Field
      key={key}
      label={label}
      placeholder={placeholder}
      value={form.values[key]}
      onChange={(event) => form.setField(key, event.target.value)}
      error={key === 'mainOkvedCode' ? errors.mainOkvedCode : undefined}
      hint={key === 'ftsCountryName' ? 'Если известно, укажите страны через запятую.' : undefined}
      disabled={disabled}
    />
  ))
}

function EnumFields({ definitions, form, disabled }: FieldsProps & { definitions: EnumDefinition[] }) {
  return definitions.map(({ key, label, options }) => (
    <Select key={key} label={label} options={[{ value: '', label: 'Не указано' }, ...options]}
      value={form.values[key]}
      onChange={(event) => form.setField(key, event.target.value as ProfileFormValues[EnumKey])}
      disabled={disabled} />
  ))
}

function BooleanFields({ definitions, form, disabled }: FieldsProps & { definitions: BooleanDefinition[] }) {
  return definitions.map(({ key, label }) => (
    <Select
      key={key}
      label={label}
      options={YES_NO_OPTIONS}
      value={form.values[key]}
      onChange={(event) => form.setField(key, event.target.value as TriState)}
      disabled={disabled}
    />
  ))
}

function SectionHeader({ title, description, badge }: { title: string; description: string; badge: string }) {
  return (
    <div className={styles.sectionHeader}>
      <div className={styles.sectionHeadings}>
        <h2 className={styles.sectionTitle}>{title}</h2>
        <p className={styles.sectionDescription}>{description}</p>
      </div>
      <Badge>{badge}</Badge>
    </div>
  )
}

/** Полная форма соответствует редактируемым полям Domain API. */
export function ProfileSectionFields({ footer, syntheticU, ...props }: FieldsProps & { footer?: ReactNode }) {
  return (
    <div className={styles.sections}>
      <SectionHeader title="Основные данные" description="Сведения об организации и её деятельности." badge="Профиль" />
      {syntheticU && (
        <Field label="Идентификатор компании в наборе Альфы" value={syntheticU} readOnly
          hint="Идентификатор назначается при импорте и не меняется." />
      )}
      <div className={styles.grid}>
        <Field label="Форма организации" value="Коммерческая организация" disabled
          hint="В наборе представлены коммерческие юридические лица." />
        <EnumFields definitions={enumFields.slice(0, 3)} {...props} />
        <TextFields definitions={basicFields} {...props} />
      </div>
      <hr className={styles.divider} />

      <SectionHeader title="Размер и связи" description="Численность, выручка и группа компаний." badge="Масштаб" />
      <div className={styles.grid}>
        <EnumFields definitions={enumFields.slice(3)} {...props} />
        <TextFields definitions={scaleFields} {...props} />
        <Field label="Годовая выручка, рублей" placeholder="1 000 000"
          inputMode="numeric" value={props.form.values.revenue}
          onChange={(event) => props.form.setRevenue(event.target.value)}
          error={props.errors.revenue} disabled={props.disabled} />
      </div>
      <hr className={styles.divider} />

      <SectionHeader title="Клиент и продукты" description="Статус клиента и подключённые банковские продукты." badge="Банк" />
      <div className={styles.grid}>
        <Select label="Отношения с банком" options={CLIENT_TYPE_OPTIONS}
          value={props.form.values.clientType}
          onChange={(event) => props.form.setField('clientType', event.target.value as ProfileFormValues['clientType'])}
          disabled={props.disabled} />
        <BooleanFields definitions={[{ key: 'negativeMark', label: 'Есть отметка о риске ключевого собственника?' }]} {...props} />
        <BooleanFields definitions={productFields} {...props} />
      </div>
      <hr className={styles.divider} />

      <SectionHeader title="Внешняя торговля" description="Данные Федеральной таможенной службы." badge="ФТС" />
      <div className={styles.grid}>
        <BooleanFields definitions={[{ key: 'ftsFlag', label: 'Компания ведёт внешнюю торговлю?' }]} {...props} />
        <TextFields definitions={[{ key: 'ftsCountryName', label: 'Страны внешней торговли' }]} {...props} />
      </div>
      <hr className={styles.divider} />

      <SectionHeader title="Местонахождение" description="Город и регион регистрации компании." badge="Адрес" />
      <div className={styles.grid}>
        <TextFields definitions={[
          { key: 'city', label: 'Город регистрации', placeholder: 'Новосибирск' },
          { key: 'region', label: 'Регион регистрации', placeholder: 'Новосибирская область' },
        ]} {...props} />
      </div>
      {footer}
    </div>
  )
}

export function ProfileGridFields(props: FieldsProps) {
  return <ProfileSectionFields {...props} />
}

/** Выбор между вводом всех полей и импортом по идентификатору CSV. */
export function ProfileCreationFields({ mode, onModeChange, syntheticUInput, onSyntheticUChange,
  syntheticUError, ...props }: CreationProps) {
  return (
    <div className={styles.sections}>
      <Select label="Как заполнить профиль" options={[
        { value: 'manual', label: 'Ввести вручную' },
        { value: 'dataset', label: 'Найти данные в наборе Альфы' },
      ]} value={mode} onChange={(event) => onModeChange(event.target.value as CreationMode)}
        disabled={props.disabled} />
      {mode === 'dataset' ? (
        <Field label="Идентификатор компании в наборе Альфы" placeholder="Например, U_67" value={syntheticUInput}
          onChange={(event) => onSyntheticUChange(event.target.value)}
          hint="Данные компании будут загружены из набора Альфы. После создания их можно отредактировать."
          error={syntheticUError} disabled={props.disabled} />
      ) : <ProfileGridFields {...props} />}
    </div>
  )
}
