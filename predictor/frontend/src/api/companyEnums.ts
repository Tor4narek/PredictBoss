/** Справочные значения профиля, которые реально встречаются в CSV Альфы. */
export enum OrganizationType {
  Commercial = 'КОММ_ОРГ',
}

export enum OwnershipForm {
  Private = 'ЧАСТНАЯ СОБСТВЕННОСТЬ',
  JointPrivateAndForeign = 'СОВМЕСТНАЯ ЧАСТНАЯ И ИНОСТРАННАЯ СОБСТВЕННОСТЬ',
  ForeignIndividuals = 'СОБСТВЕННОСТЬ ИНОСТРАННЫХ ГРАЖДАН И ЛИЦ БЕЗ ГРАЖДАНСТВА',
  ForeignLegalEntities = 'СОБСТВЕННОСТЬ ИНОСТРАННЫХ ЮРИДИЧЕСКИХ ЛИЦ',
  Federal = 'ФЕДЕРАЛЬНАЯ СОБСТВЕННОСТЬ',
  OtherMixedRussian = 'ИНАЯ СМЕШАННАЯ РОССИЙСКАЯ СОБСТВЕННОСТЬ',
  MixedRussianWithFederalShare = 'СМЕШАННАЯ РОССИЙСКАЯ СОБСТВЕННОСТЬ С ДОЛЕЙ ФЕДЕРАЛЬНОЙ СОБСТВЕННОСТИ',
  MixedRussianWithRegionalShare = 'СМЕШАННАЯ РОССИЙСКАЯ СОБСТВЕННОСТЬ С ДОЛЕЙ СОБСТВЕННОСТИ СУБЪЕКТОВ РОССИЙСКОЙ ФЕДЕРАЦИИ',
  Regional = 'СОБСТВЕННОСТЬ СУБЪЕКТОВ РОССИЙСКОЙ ФЕДЕРАЦИИ',
  MixedForeign = 'СМЕШАННАЯ ИНОСТРАННАЯ СОБСТВЕННОСТЬ',
  Municipal = 'МУНИЦИПАЛЬНАЯ СОБСТВЕННОСТЬ',
  PublicAssociations = 'СОБСТВЕННОСТЬ ОБЩЕСТВЕННЫХ ОБЪЕДИНЕНИЙ',
}

export enum CompanyAgeGradation {
  UpTo3Years = '0-3',
  From4To7Years = '4-7',
  From8To15Years = '8-15',
  From16To25Years = '16-25',
}

export enum BusinessDevelopmentStage {
  Birth = 'Рождение',
  RapidGrowth = 'Стремительный рост',
  Optimization = 'Оптимизация',
  NewReality = 'Бизнес в новой реальности',
}

export enum EmployeeCountGradation {
  Under15 = 'Менее 15',
  From16To100 = '16-100',
  From101To250 = '101-250',
  Over251 = 'Более 251',
}

export enum ClientType {
  Prospect = 'PROSPECT',
  Current = 'CLIENT',
  Former = 'CLOSE_CLIENT',
}

export function ownershipFormLabel(value: OwnershipForm): string {
  const sentence = value.toLocaleLowerCase('ru-RU')
  return sentence.charAt(0).toLocaleUpperCase('ru-RU') + sentence.slice(1)
}
