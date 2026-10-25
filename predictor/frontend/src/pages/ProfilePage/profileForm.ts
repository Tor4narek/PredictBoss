import { useState } from 'react'
import type { CompanyProfile, CompanyProfileInput } from '../../api'
import { formatNumber } from '../../lib/format'
import type { SelectOption } from '../../ui'

/* Логика формы профиля: общая для страницы профиля и мастера создания компании. */

export type ProfileFormValues = {
  orgType: string
  mainOkvedCode: string
  mainOkvedName: string
  region: string
  /** Выручка с пробелами между разрядами: «18 000 000 000». */
  revenue: string
  hasLoans: '' | 'yes' | 'no'
}

export const ORG_TYPE_OPTIONS: SelectOption[] = [
  { value: '', label: 'Не указано' },
  { value: 'Юридическое лицо', label: 'Юридическое лицо' },
  { value: 'Индивидуальный предприниматель', label: 'Индивидуальный предприниматель' },
]

export const YES_NO_OPTIONS: SelectOption[] = [
  { value: '', label: 'Не указано' },
  { value: 'yes', label: 'Да' },
  { value: 'no', label: 'Нет' },
]

export function toFormValues(profile: CompanyProfile | null | undefined): ProfileFormValues {
  return {
    orgType: profile?.orgType ?? '',
    mainOkvedCode: profile?.mainOkvedCode ?? '',
    mainOkvedName: profile?.mainOkvedName ?? '',
    region: profile?.region ?? '',
    revenue: profile?.revenueCompany != null ? formatNumber(profile.revenueCompany) : '',
    hasLoans: profile?.hasLoans == null ? '' : profile.hasLoans ? 'yes' : 'no',
  }
}

export function toProfileInput(values: ProfileFormValues): CompanyProfileInput {
  const text = (value: string) => value.trim() || null
  const digits = values.revenue.replace(/\D/g, '')
  return {
    orgType: text(values.orgType),
    mainOkvedCode: text(values.mainOkvedCode),
    mainOkvedName: text(values.mainOkvedName),
    region: text(values.region),
    revenueCompany: digits ? Number(digits) : null,
    hasLoans: values.hasLoans === '' ? null : values.hasLoans === 'yes',
  }
}

/** Ошибки полей. Пустой объект — можно сохранять. */
export function validateProfile(values: ProfileFormValues): {
  form?: string
  mainOkvedCode?: string
} {
  const input = toProfileInput(values)
  if (Object.values(input).every((value) => value === null)) {
    return { form: 'Заполните хотя бы одно поле — без данных анализ невозможен.' }
  }
  if (input.mainOkvedCode && !/^\d{2}(\.\d{1,2}){0,2}$/.test(input.mainOkvedCode)) {
    return { mainOkvedCode: 'Код ОКВЭД в формате 46.90' }
  }
  return {}
}

/** Значения формы и функции для их изменения. */
export function useProfileForm(profile: CompanyProfile | null | undefined) {
  const [values, setValues] = useState(() => toFormValues(profile))
  const [initial, setInitial] = useState(values)

  function setField<K extends keyof ProfileFormValues>(key: K, value: ProfileFormValues[K]) {
    setValues((current) => ({ ...current, [key]: value }))
  }

  /** Выручка: оставляем только цифры и разбиваем по разрядам. */
  function setRevenue(raw: string) {
    const digits = raw.replace(/\D/g, '').slice(0, 15)
    setField('revenue', digits ? formatNumber(Number(digits)) : '')
  }

  /** Сбросить к значениям профиля (после «Отменить» или сохранения). */
  function reset(next: CompanyProfile | null | undefined = profile) {
    const nextValues = toFormValues(next)
    setValues(nextValues)
    setInitial(nextValues)
  }

  const dirty = JSON.stringify(values) !== JSON.stringify(initial)

  return { values, setField, setRevenue, reset, dirty }
}

export type ProfileForm = ReturnType<typeof useProfileForm>
