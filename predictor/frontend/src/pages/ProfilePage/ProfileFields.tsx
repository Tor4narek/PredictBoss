import type { ReactNode } from 'react'
import { Badge, Field, Select } from '../../ui'
import { ORG_TYPE_OPTIONS, YES_NO_OPTIONS, type ProfileForm } from './profileForm'
import styles from './ProfilePage.module.css'

type FieldsProps = {
  form: ProfileForm
  errors: { mainOkvedCode?: string }
  disabled?: boolean
}

/* Поля профиля. Каждое поле — компонент из UI kit, раскладки взяты из фреймов 04 и 14. */

function OrgTypeField({ form, disabled }: FieldsProps) {
  return (
    <Select
      label="Тип организации"
      options={ORG_TYPE_OPTIONS}
      value={form.values.orgType}
      onChange={(event) => form.setField('orgType', event.target.value)}
      disabled={disabled}
    />
  )
}

function OkvedCodeField({ form, errors, disabled }: FieldsProps) {
  return (
    <Field
      label="Код ОКВЭД"
      placeholder="46.90"
      inputMode="decimal"
      value={form.values.mainOkvedCode}
      onChange={(event) => form.setField('mainOkvedCode', event.target.value)}
      error={errors.mainOkvedCode}
      disabled={disabled}
    />
  )
}

function OkvedNameField({ form, disabled }: FieldsProps) {
  return (
    <Field
      label="Название ОКВЭД"
      placeholder="Торговля оптовая неспециализированная"
      value={form.values.mainOkvedName}
      onChange={(event) => form.setField('mainOkvedName', event.target.value)}
      disabled={disabled}
    />
  )
}

function RegionField({ form, disabled }: FieldsProps) {
  return (
    <Field
      label="Регион"
      placeholder="Москва"
      value={form.values.region}
      onChange={(event) => form.setField('region', event.target.value)}
      disabled={disabled}
    />
  )
}

function RevenueField({ form, disabled }: FieldsProps) {
  return (
    <Field
      label="Годовая выручка, ₽"
      placeholder="18 000 000 000"
      inputMode="numeric"
      value={form.values.revenue}
      onChange={(event) => form.setRevenue(event.target.value)}
      disabled={disabled}
    />
  )
}

function LoansField({ form, disabled }: FieldsProps) {
  return (
    <Select
      label="Кредиты"
      options={YES_NO_OPTIONS}
      value={form.values.hasLoans}
      onChange={(event) =>
        form.setField('hasLoans', event.target.value as ProfileForm['values']['hasLoans'])
      }
      disabled={disabled}
    />
  )
}

/** Плоская форма в две колонки (фреймы «04 · Ручной профиль» и «02 · Создание компании»). */
export function ProfileGridFields(props: FieldsProps) {
  return (
    <div className={styles.grid}>
      <OkvedCodeField {...props} />
      <OkvedNameField {...props} />
      <RegionField {...props} />
      <RevenueField {...props} />
      <OrgTypeField {...props} />
      <LoansField {...props} />
    </div>
  )
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

/** Форма по разделам (фрейм «14 · Профиль компании»). footer — строка с кнопками внизу. */
export function ProfileSectionFields({ footer, ...props }: FieldsProps & { footer: ReactNode }) {
  return (
    <div className={styles.sections}>
      <SectionHeader
        title="Основные данные"
        description="Тип организации и основной вид деятельности."
        badge="Основное"
      />
      <div className={styles.row3}>
        <OrgTypeField {...props} />
        <OkvedCodeField {...props} />
        <OkvedNameField {...props} />
      </div>
      <hr className={styles.divider} />

      <SectionHeader
        title="Контакты и структура"
        description="География и базовые параметры клиента."
        badge="Профиль"
      />
      <RegionField {...props} />
      <hr className={styles.divider} />

      <SectionHeader
        title="Банковские продукты"
        description="Продукты банка, которыми пользуется компания."
        badge="Булевы признаки"
      />
      <LoansField {...props} />
      <hr className={styles.divider} />

      <SectionHeader
        title="Финансовые показатели"
        description="Помогают оценить масштаб влияния событий."
        badge="Финансы"
      />
      <RevenueField {...props} />
      <hr className={styles.divider} />

      {footer}
    </div>
  )
}
