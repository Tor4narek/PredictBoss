import { useState } from 'react'
import type { CompanyProfile, CompanyProfileInput } from '../../api'
import {
  BusinessDevelopmentStage,
  ClientType,
  CompanyAgeGradation,
  EmployeeCountGradation,
  OrganizationType,
  OwnershipForm,
} from '../../api/companyEnums'
import { formatNumber } from '../../lib/format'
import type { SelectOption } from '../../ui'

export type TriState = '' | 'yes' | 'no'
/** Все редактируемые поля Domain; syntheticU назначает только импорт CSV. */
export type ProfileFormValues = {
  orgType: OrganizationType | ''
  ownershipForm: OwnershipForm | ''
  companyAgeGradation: CompanyAgeGradation | ''
  developmentStage: BusinessDevelopmentStage | ''
  mainOkvedCode: string
  mainOkvedName: string
  negativeMark: TriState
  workersFnsGradation: EmployeeCountGradation | ''
  workersSparkGradation: EmployeeCountGradation | ''
  clientType: '' | ClientType
  hasRko: TriState
  hasGuarantees: TriState
  hasLoans: TriState
  hasLeasing: TriState
  hasDeposits: TriState
  hasPayroll: TriState
  ftsFlag: TriState
  ftsCountryName: string
  hasFactoring: TriState
  revenue: string
  groupId: string
  city: string
  region: string
}

export const YES_NO_OPTIONS: SelectOption[] = [
  { value: '', label: 'Не указано' },
  { value: 'yes', label: 'Да' },
  { value: 'no', label: 'Нет' },
]

export const CLIENT_TYPE_OPTIONS: SelectOption[] = [
  { value: '', label: 'Не указано' },
  { value: ClientType.Prospect, label: 'Потенциальный клиент' },
  { value: ClientType.Current, label: 'Действующий клиент' },
  { value: ClientType.Former, label: 'Бывший клиент' },
]

const triState = (value: boolean | null | undefined): TriState =>
  value == null ? '' : value ? 'yes' : 'no'
const nullableBoolean = (value: TriState): boolean | null => value === '' ? null : value === 'yes'
const nullableText = <T extends string>(value: T): Exclude<T, ''> | null =>
  value.trim() ? value.trim() as Exclude<T, ''> : null

export function toFormValues(profile: CompanyProfile | null | undefined): ProfileFormValues {
  return {
    orgType: profile?.orgType ?? OrganizationType.Commercial,
    ownershipForm: profile?.ownershipForm ?? '',
    companyAgeGradation: profile?.companyAgeGradation ?? '',
    developmentStage: profile?.developmentStage ?? '',
    mainOkvedCode: profile?.mainOkvedCode ?? '',
    mainOkvedName: profile?.mainOkvedName ?? '',
    negativeMark: triState(profile?.negativeMark),
    workersFnsGradation: profile?.workersFnsGradation ?? '',
    workersSparkGradation: profile?.workersSparkGradation ?? '',
    clientType: profile?.clientType ?? '',
    hasRko: triState(profile?.hasRko),
    hasGuarantees: triState(profile?.hasGuarantees),
    hasLoans: triState(profile?.hasLoans),
    hasLeasing: triState(profile?.hasLeasing),
    hasDeposits: triState(profile?.hasDeposits),
    hasPayroll: triState(profile?.hasPayroll),
    ftsFlag: triState(profile?.ftsFlag),
    ftsCountryName: profile?.ftsCountryName ?? '',
    hasFactoring: triState(profile?.hasFactoring),
    revenue: profile?.revenueCompany != null ? formatNumber(profile.revenueCompany) : '',
    groupId: profile?.groupId ?? '',
    city: profile?.city ?? '',
    region: profile?.region ?? '',
  }
}

export function toProfileInput(values: ProfileFormValues): CompanyProfileInput {
  const digits = values.revenue.replace(/\s/g, '')
  return {
    orgType: nullableText(values.orgType),
    ownershipForm: nullableText(values.ownershipForm),
    companyAgeGradation: nullableText(values.companyAgeGradation),
    developmentStage: nullableText(values.developmentStage),
    mainOkvedCode: nullableText(values.mainOkvedCode),
    mainOkvedName: nullableText(values.mainOkvedName),
    negativeMark: nullableBoolean(values.negativeMark),
    workersFnsGradation: nullableText(values.workersFnsGradation),
    workersSparkGradation: nullableText(values.workersSparkGradation),
    clientType: values.clientType || null,
    hasRko: nullableBoolean(values.hasRko),
    hasGuarantees: nullableBoolean(values.hasGuarantees),
    hasLoans: nullableBoolean(values.hasLoans),
    hasLeasing: nullableBoolean(values.hasLeasing),
    hasDeposits: nullableBoolean(values.hasDeposits),
    hasPayroll: nullableBoolean(values.hasPayroll),
    ftsFlag: nullableBoolean(values.ftsFlag),
    ftsCountryName: nullableText(values.ftsCountryName),
    hasFactoring: nullableBoolean(values.hasFactoring),
    revenueCompany: digits ? Number(digits) : null,
    groupId: nullableText(values.groupId),
    city: nullableText(values.city),
    region: nullableText(values.region),
  }
}

export function validateProfile(values: ProfileFormValues): {
  form?: string
  mainOkvedCode?: string
  revenue?: string
} {
  const input = toProfileInput(values)
  const { orgType: _orgType, ...userProvidedFields } = input
  if (Object.values(userProvidedFields).every((value) => value === null)) {
    return { form: 'Заполните хотя бы одно поле — без данных анализ невозможен.' }
  }
  if (input.mainOkvedCode && !/^\d{2}(\.\d{1,2}){0,2}$/.test(input.mainOkvedCode)) {
    return { mainOkvedCode: 'Код ОКВЭД в формате 46.90 или 46.34.2.' }
  }
  if (values.revenue && (!/^\d{1,15}$/.test(values.revenue.replace(/\s/g, '')) ||
    !Number.isSafeInteger(input.revenueCompany ?? NaN))) {
    return { revenue: 'Введите целое число рублей не длиннее 15 цифр.' }
  }
  return {}
}

export function useProfileForm(profile: CompanyProfile | null | undefined) {
  const [values, setValues] = useState(() => toFormValues(profile))
  const [initial, setInitial] = useState(values)

  function setField<K extends keyof ProfileFormValues>(key: K, value: ProfileFormValues[K]) {
    setValues((current) => ({ ...current, [key]: value }))
  }

  function setRevenue(raw: string) {
    const digits = raw.replace(/\D/g, '').slice(0, 15)
    setField('revenue', digits ? formatNumber(Number(digits)) : '')
  }

  function reset(next: CompanyProfile | null | undefined = profile) {
    const nextValues = toFormValues(next)
    setValues(nextValues)
    setInitial(nextValues)
  }

  const dirty = JSON.stringify(values) !== JSON.stringify(initial)
  return { values, setField, setRevenue, reset, dirty }
}

export type ProfileForm = ReturnType<typeof useProfileForm>
